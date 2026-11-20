package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/modules/tenants"
	"github.com/school-without-walls/api/internal/platform/db"
	"github.com/school-without-walls/api/migrations"
)

func staffHTTPTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if dsn == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL is required")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to the isolated PostgreSQL test database")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid SWW_TEST_DATABASE_URL")
	}
	c := cfg.ConnConfig
	if c.Database != "sww_structure_test" ||
		(c.Port != 5432 && dsn != "postgres://sww_browser@127.0.0.1:15432/sww_structure_test?sslmode=disable") ||
		(c.Host != "localhost" && c.Host != "127.0.0.1") {
		t.Fatal("staff HTTP tests require local sww_structure_test or the dedicated disposable fixture")
	}
	for _, fallback := range c.Fallbacks {
		if fallback.Host != c.Host || fallback.Port != c.Port {
			t.Fatal("unexpected PostgreSQL fallback address")
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal("cannot initialize test database pool")
	}
	t.Cleanup(admin.Close)
	schema := "sww_staff_http_" + uuid.New().String()
	schema = strings.ReplaceAll(schema, "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("cannot create isolated staff HTTP test schema")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("cannot clean up staff HTTP test schema")
		}
	})
	if c.RuntimeParams == nil {
		c.RuntimeParams = make(map[string]string)
	}
	c.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("cannot initialize schema-scoped test pool")
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (
		version varchar(64) PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func newStaffTestRouter(t *testing.T, pool *pgxpool.Pool, origin, webURL string, limits config.StaffInvitationLimits) http.Handler {
	t.Helper()
	handler, err := NewRouter(RouterConfig{
		Config: &config.Config{Environment: "test", PublicWebURL: webURL, CORSAllowedOrigins: []string{origin}, StaffInvitations: limits},
		DB:     &db.DB{Pool: pool},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestPostgresStaffInvitationHTTP(t *testing.T) {
	pool := staffHTTPTestPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	tenantA, tenantB := uuid.New(), uuid.New()
	exec(`INSERT INTO tenants (id,slug,name,status) VALUES
		($1,'staff-test-a','Test School A','ACTIVE'), ($2,'staff-test-b','Test School B','ACTIVE')`, tenantA, tenantB)

	type credentials struct{ session, csrf string }
	actors := map[string]credentials{}
	ids := map[string]uuid.UUID{}
	repo := auth.NewPostgresRepository(pool)
	for _, actor := range []struct{ name, role, status string }{
		{"owner", "SCHOOL_OWNER", "ACTIVE"},
		{"suspended-owner", "SCHOOL_OWNER", "SUSPENDED"},
		{"teacher", "TEACHER", "ACTIVE"},
		{"admin", "SCHOOL_ADMIN", "ACTIVE"},
		{"principal", "PRINCIPAL", "ACTIVE"},
		{"platform", "", ""},
	} {
		id := uuid.New()
		ids[actor.name] = id
		exec(`INSERT INTO users (id,email,password_hash,full_name,status)
			VALUES ($1,$2,'unused-test-hash','Fictional Staff','ACTIVE')`, id, actor.name+"@school.invalid")
		if actor.role != "" {
			exec(`INSERT INTO memberships (id,tenant_id,user_id,role,status)
				VALUES ($1,$2,$3,$4,$5)`, uuid.New(), tenantA, id, actor.role, actor.status)
		}
		cred := credentials{uuid.NewString(), uuid.NewString()}
		actors[actor.name] = cred
		if err := repo.CreateSession(ctx, &auth.Session{
			ID: uuid.New(), UserID: id, TokenHash: auth.HashToken(cred.session),
			CSRFTokenHash: auth.HashToken(cred.csrf), ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO platform_admins (user_id) VALUES ($1)", ids["platform"])
	const origin = "http://localhost:3000"
	newRouter := func(webURL string) http.Handler {
		return newStaffTestRouter(t, pool, origin, webURL, config.StaffInvitationLimits{})
	}
	// Construct a separate API instance over the same authoritative database.
	limitedRouter := newStaffTestRouter(t, pool, origin, origin, config.StaffInvitationLimits{PerActor: 1, PerTenant: 2, Window: time.Hour})
	router := newRouter(origin)
	send := func(handler http.Handler, method, path, body string, cred credentials) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if cred.session != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cred.session})
		}
		req.Header.Set(auth.HeaderCSRF, cred.csrf)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	messages := func(t *testing.T, handler http.Handler) []tenants.CaptureMessage {
		t.Helper()
		res := send(handler, "GET", "/api/v1/platform/mail-capture", "", actors["platform"])
		var envelope struct {
			Data struct{ Messages []tenants.CaptureMessage }
		}
		if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" ||
			json.Unmarshal(res.Body.Bytes(), &envelope) != nil {
			t.Fatalf("cannot read captured mail, status %d", res.Code)
		}
		return envelope.Data.Messages
	}
	type counts struct{ invitations, sessions, memberships, users, audits int }
	snapshot := func(t *testing.T) counts {
		t.Helper()
		var c counts
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM invitations),
			(SELECT count(*) FROM sessions), (SELECT count(*) FROM memberships),
			(SELECT count(*) FROM users), (SELECT count(*) FROM audit_events)`).
			Scan(&c.invitations, &c.sessions, &c.memberships, &c.users, &c.audits); err != nil {
			t.Fatal(err)
		}
		return c
	}
	const path = "/api/v1/tenants/staff-test-a/staff-invitations"
	const otherPath = "/api/v1/tenants/staff-test-b/staff-invitations"
	body := func(email, role string) string {
		return fmt.Sprintf(`{"email":%q,"role":%q}`, email, role)
	}
	tokenFromMail := func(t *testing.T, message tenants.CaptureMessage) string {
		t.Helper()
		_, rest, ok := strings.Cut(message.Body, origin+"/invite/")
		token, _, _ := strings.Cut(rest, "\r")
		token, _, _ = strings.Cut(token, "\n")
		if !ok || token == "" {
			t.Fatal("captured mail has no invitation link")
		}
		return token
	}
	const inviteePassword = "fictional onboarding password 42!"
	accept := func(token string) *httptest.ResponseRecorder {
		return send(router, "POST", "/api/v1/invitations/accept",
			fmt.Sprintf(`{"token":%q,"full_name":"Fictional Invitee","password":%q}`, token, inviteePassword), credentials{})
	}
	readCredentials := func(t *testing.T, res *httptest.ResponseRecorder) credentials {
		t.Helper()
		var envelope struct{ Data auth.LoginResponseData }
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal("invalid authenticated response")
		}
		for _, cookie := range res.Result().Cookies() {
			if cookie.Name == auth.CookieName && cookie.Value != "" && cookie.HttpOnly &&
				envelope.Data.CSRFToken != "" && !strings.Contains(res.Body.String(), cookie.Value) {
				return credentials{cookie.Value, envelope.Data.CSRFToken}
			}
		}
		t.Fatal("expected an HttpOnly session cookie and a separate CSRF token")
		return credentials{}
	}
	reject := func(t *testing.T, route, payload string, cred credentials, status int) {
		t.Helper()
		before := snapshot(t)
		mailBefore := len(messages(t, router))
		res := send(router, "POST", route, payload, cred)
		if res.Code != status {
			t.Fatalf("status %d, want %d", res.Code, status)
		}
		if snapshot(t) != before || len(messages(t, router)) != mailBefore || len(res.Result().Cookies()) != 0 {
			t.Fatal("rejected invitation request had side effects")
		}
	}
	t.Run("authenticated non-owner denial does not wait on the school lock", func(t *testing.T) {
		before := snapshot(t)
		mailBefore := len(messages(t, router))
		blocker, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback(ctx)
		if _, err := blocker.Exec(ctx, "SELECT id FROM tenants WHERE id=$1 FOR UPDATE", tenantB); err != nil {
			t.Fatal(err)
		}
		result := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			result <- send(router, "POST", otherPath, body("staff-lock-denied@school.invalid", "PRINCIPAL"), actors["owner"])
		}()
		wait, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		select {
		case response := <-result:
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", response.Code)
			}
		case <-wait.Done():
			t.Fatal("non-owner was blocked by the other school's row lock")
		}
		if snapshot(t) != before || len(messages(t, router)) != mailBefore {
			t.Fatal("denied request performed an issuance side effect")
		}
	})
	valid := body("new-staff@school.invalid", "SCHOOL_ADMIN")
	for _, tc := range []struct {
		name, path, body string
		cred             credentials
		status           int
	}{
		{"anonymous", path, valid, credentials{}, 401},
		{"missing CSRF", path, valid, credentials{session: actors["owner"].session}, 403},
		{"invalid CSRF", path, valid, credentials{actors["owner"].session, actors["teacher"].csrf}, 403},
		{"other school", otherPath, valid, actors["owner"], 403},
		{"suspended owner", path, valid, actors["suspended-owner"], 403},
		{"invalid email", path, body("bad-email", "PRINCIPAL"), actors["owner"], 422},
		{"malformed JSON", path, "{", actors["owner"], 422},
		{"forbidden owner grant", path, body("new-staff@school.invalid", "SCHOOL_OWNER"), actors["owner"], 422},
		{"forbidden teacher grant", path, body("new-staff@school.invalid", "TEACHER"), actors["owner"], 422},
		{"matching membership", path, body("principal@school.invalid", "PRINCIPAL"), actors["owner"], 409},
	} {
		t.Run(tc.name, func(t *testing.T) { reject(t, tc.path, tc.body, tc.cred, tc.status) })
	}
	for _, actor := range []string{"teacher", "admin", "principal", "platform"} {
		for _, role := range []string{"SCHOOL_ADMIN", "PRINCIPAL"} {
			t.Run(actor+" cannot grant "+role, func(t *testing.T) {
				reject(t, path, body("new-staff@school.invalid", role), actors[actor], 403)
			})
		}
	}
	for _, staff := range []struct{ role, label, email string }{
		{"SCHOOL_ADMIN", "School Administrator", "new-admin@school.invalid"},
		{"PRINCIPAL", "Principal", "new-principal@school.invalid"},
	} {
		t.Run(staff.role+" invitation", func(t *testing.T) {
			before := snapshot(t)
			mailBefore := len(messages(t, router))
			payload := body(staff.email, staff.role)
			res := send(router, "POST", path, payload, actors["owner"])
			var response struct{ Data tenants.StaffInvitationResult }
			if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &response) != nil ||
				response.Data.ID == uuid.Nil || response.Data.Role != staff.role || response.Data.Email != staff.email {
				t.Fatalf("invalid create response, status %d", res.Code)
			}
			mail := messages(t, router)
			if len(mail) != mailBefore+1 || mail[mailBefore].To != staff.email ||
				!strings.Contains(mail[mailBefore].Subject, "Invitation to join Test School A") ||
				!strings.Contains(mail[mailBefore].Body, "as a "+staff.label+".") {
				t.Fatal("expected exactly one role-specific invitation email")
			}
			token := tokenFromMail(t, mail[mailBefore])
			verifyPath := "/api/v1/invitations/" + token + "/verify"
			verified := send(router, "GET", verifyPath, "", credentials{})
			var details struct{ Data map[string]json.RawMessage }
			if verified.Code != 200 || verified.Header().Get("Cache-Control") != "no-store" ||
				json.Unmarshal(verified.Body.Bytes(), &details) != nil || len(details.Data) != 4 ||
				string(details.Data["invited_email"]) != fmt.Sprintf("%q", staff.email) ||
				string(details.Data["tenant_slug"]) != `"staff-test-a"` ||
				string(details.Data["tenant_name"]) != `"Test School A"` || len(details.Data["expires_at"]) == 0 ||
				strings.Contains(verified.Body.String(), token) {
				t.Fatalf("invalid invitation verification response, status %d", verified.Code)
			}
			var persisted bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM invitations i JOIN audit_events a ON a.subject_id=i.id::text
				WHERE i.id=$1 AND i.tenant_id=$2 AND i.created_by=$3 AND i.invited_email=$4
				AND i.target_role=$5 AND i.token_hash=$6 AND i.status='PENDING'
				AND a.tenant_id=$2 AND a.actor_id=$3 AND a.subject_type='invitation'
				AND a.action='tenant.staff_invitation.create'
			)`, response.Data.ID, tenantA, ids["owner"], staff.email, staff.role, auth.HashToken(token)).
				Scan(&persisted); err != nil {
				t.Fatal(err)
			}
			before.invitations++
			before.audits++
			if !persisted || snapshot(t) != before || strings.Contains(res.Body.String(), token) {
				t.Fatal("invalid invitation persistence, audit, response or side effects")
			}
			reject(t, path, payload, actors["owner"], 409)
			t.Run("single use under concurrent acceptance", func(t *testing.T) {
				start := make(chan struct{})
				results := make(chan *httptest.ResponseRecorder, 2)
				for range 2 {
					go func() {
						<-start
						results <- accept(token)
					}()
				}
				close(start)
				first, second := <-results, <-results
				if first.Code == 422 {
					first, second = second, first
				}
				if first.Code != 201 || second.Code != 422 || len(second.Result().Cookies()) != 0 {
					t.Fatalf("concurrent acceptance statuses %d and %d, want 201 and 422", first.Code, second.Code)
				}
				if first.Header().Get("Cache-Control") != "no-store" ||
					strings.Contains(first.Body.String(), token) || strings.Contains(first.Body.String(), inviteePassword) {
					t.Fatal("acceptance exposed credentials or was cacheable")
				}
				if me := send(router, "GET", "/api/v1/auth/me", "", readCredentials(t, first)); me.Code != 200 {
					t.Fatalf("acceptance did not establish an authenticated session, status %d", me.Code)
				}
				before.users++
				before.memberships++
				before.sessions++
				before.audits++
				var accepted bool
				if err := pool.QueryRow(ctx, `SELECT EXISTS (
					SELECT 1 FROM invitations i JOIN memberships m ON m.tenant_id=i.tenant_id
					JOIN users u ON u.id=m.user_id AND u.email=i.invited_email
					WHERE i.id=$1 AND i.status='ACCEPTED' AND i.used_at IS NOT NULL
					AND m.tenant_id=$2 AND m.role=$3 AND m.status='ACTIVE'
				)`, response.Data.ID, tenantA, staff.role).Scan(&accepted); err != nil {
					t.Fatal(err)
				}
				if !accepted || snapshot(t) != before {
					t.Fatal("acceptance did not grant exactly one tenant membership and session")
				}
				if res := accept(token); res.Code != 422 || snapshot(t) != before {
					t.Fatalf("replayed token status %d or unexpected side effects", res.Code)
				}
			})
			t.Run("staff login and school authorization", func(t *testing.T) {
				login := send(router, "POST", "/api/v1/auth/login",
					fmt.Sprintf(`{"email":%q,"password":%q}`, staff.email, inviteePassword), credentials{})
				if login.Code != 200 {
					t.Fatalf("staff login status %d", login.Code)
				}
				cred := readCredentials(t, login)
				schools := send(router, "GET", "/api/v1/my/schools", "", cred)
				var membership struct {
					Data struct{ Schools []tenants.Tenant }
				}
				if schools.Code != 200 || json.Unmarshal(schools.Body.Bytes(), &membership) != nil ||
					len(membership.Data.Schools) != 1 || membership.Data.Schools[0].ID != tenantA ||
					membership.Data.Schools[0].CurrentRole != staff.role {
					t.Fatalf("staff dashboard received the wrong memberships, status %d", schools.Code)
				}
				for _, route := range []struct {
					path   string
					status int
				}{
					{"/api/v1/tenants/staff-test-a/settings", 200},
					{"/api/v1/tenants/staff-test-a/academic-sessions", 200},
					{"/api/v1/tenants/staff-test-b/settings", 404},
					{"/api/v1/platform/mail-capture", 403},
				} {
					if res := send(router, "GET", route.path, "", cred); res.Code != route.status {
						t.Fatalf("%s: status %d, want %d", route.path, res.Code, route.status)
					}
				}
				reject(t, path, body("unauthorized-delegate@school.invalid", "PRINCIPAL"), cred, 403)

				otherRole := "SCHOOL_ADMIN"
				if staff.role == otherRole {
					otherRole = "PRINCIPAL"
				}
				mailBefore := len(messages(t, router))
				if res := send(router, "POST", path, body(staff.email, otherRole), actors["owner"]); res.Code != 201 {
					t.Fatalf("follow-up invitation status %d", res.Code)
				}
				mail := messages(t, router)
				if len(mail) != mailBefore+1 || mail[mailBefore].To != staff.email {
					t.Fatal("missing follow-up invitation email")
				}
				exec(`UPDATE memberships SET status='SUSPENDED'
					WHERE tenant_id=$1 AND role=$2 AND user_id=(SELECT id FROM users WHERE email=$3)`,
					tenantA, staff.role, staff.email)
				before := snapshot(t)
				res := accept(tokenFromMail(t, mail[mailBefore]))
				if res.Code != 422 || len(res.Result().Cookies()) != 0 || snapshot(t) != before {
					t.Fatalf("follow-up invitation bypassed a suspended membership, status %d", res.Code)
				}
				if res := send(router, "GET", "/api/v1/tenants/staff-test-a/settings", "", cred); res.Code != 404 {
					t.Fatalf("session retained access after membership suspension, status %d", res.Code)
				}
			})
		})
	}
	t.Run("delivery failure revokes invitation", func(t *testing.T) {
		handler := newRouter("invalid-web-url")
		before := snapshot(t)
		const email = "failed-delivery@school.invalid"
		res := send(handler, "POST", path, body(email, "PRINCIPAL"), actors["owner"])
		if res.Code != 500 || strings.Contains(res.Body.String(), email) || len(res.Result().Cookies()) != 0 {
			t.Fatalf("invalid delivery failure response, status %d", res.Code)
		}
		var revoked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM invitations i JOIN audit_events a ON a.subject_id=i.id::text
			WHERE i.tenant_id=$1 AND i.invited_email=$2 AND i.status='REVOKED'
			AND a.tenant_id=$1 AND a.actor_id=$3 AND a.subject_type='invitation'
			AND a.action='tenant.staff_invitation.delivery_failed'
		)`, tenantA, email, ids["owner"]).Scan(&revoked); err != nil {
			t.Fatal(err)
		}
		before.invitations++
		before.audits += 2
		if !revoked || snapshot(t) != before || len(messages(t, handler)) != 0 {
			t.Fatal("expected audited revocation with no delivery, membership or session")
		}
	})
	for _, tc := range []struct {
		name, status string
		tenantID     uuid.UUID
		validFor     time.Duration
	}{
		{"expired", "PENDING", tenantA, -time.Hour},
		{"revoked", "REVOKED", tenantA, time.Hour},
		{"suspended school", "PENDING", tenantB, time.Hour},
	} {
		t.Run("reject acceptance for "+tc.name, func(t *testing.T) {
			if tc.tenantID == tenantB {
				exec("UPDATE tenants SET status='SUSPENDED' WHERE id=$1", tenantB)
			}
			token := uuid.NewString()
			exec(`INSERT INTO invitations
				(id,tenant_id,invited_email,target_role,token_hash,status,expires_at)
				VALUES ($1,$2,'inactive-invitation@school.invalid','PRINCIPAL',$3,$4,$5)`,
				uuid.New(), tc.tenantID, auth.HashToken(token), tc.status, time.Now().Add(tc.validFor))
			before := snapshot(t)
			if tc.name == "expired" || tc.name == "revoked" {
				verified := send(router, "GET", "/api/v1/invitations/"+token+"/verify", "", credentials{})
				if verified.Code != 422 || strings.Contains(verified.Body.String(), token) {
					t.Fatalf("unusable invitation verification status %d", verified.Code)
				}
			}
			res := accept(token)
			if res.Code != 422 || len(res.Result().Cookies()) != 0 || snapshot(t) != before {
				t.Fatalf("unusable invitation status %d or unexpected side effects", res.Code)
			}
		})
	}
	t.Run("over-limit browser request creates neither invitation audit nor captured mail", func(t *testing.T) {
		before := snapshot(t)
		const recipient = "staff-over-limit@school.invalid"
		res := send(limitedRouter, "POST", path, body(recipient, "PRINCIPAL"), actors["owner"])
		retry, err := strconv.Atoi(res.Header().Get("Retry-After"))
		if res.Code != 429 || err != nil || retry < 1 || retry > 3600 ||
			!strings.Contains(res.Body.String(), "retry_after_seconds") || strings.Contains(res.Body.String(), recipient) {
			t.Fatalf("invalid quota response, status %d", res.Code)
		}
		if snapshot(t) != before || len(messages(t, limitedRouter)) != 0 {
			t.Fatal("over-limit request performed an issuance side effect")
		}
	})
	t.Run("owner provisioning and activation", func(t *testing.T) {
		before := len(messages(t, router))
		res := send(router, "POST", "/api/v1/platform/tenants",
			`{"slug":"staff-owner-regression","name":"Fictional Owner School","owner_email":"new-owner@school.invalid","locale":"en-NG","timezone":"Africa/Lagos"}`,
			actors["platform"])
		if res.Code != 201 {
			t.Fatalf("owner provisioning status %d", res.Code)
		}
		mail := messages(t, router)
		if len(mail) != before+1 || mail[before].To != "new-owner@school.invalid" ||
			mail[before].Subject != "Activate your School Without Walls school owner account" {
			t.Fatal("expected the existing owner onboarding email")
		}
		token := tokenFromMail(t, mail[before])
		res = accept(token)
		if res.Code != 201 {
			t.Fatalf("owner activation status %d", res.Code)
		}
		var active bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM tenants t JOIN memberships m ON m.tenant_id=t.id JOIN users u ON m.user_id=u.id
			WHERE t.slug='staff-owner-regression' AND t.status='ACTIVE'
			AND m.role='SCHOOL_OWNER' AND m.status='ACTIVE' AND u.email='new-owner@school.invalid'
		)`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if !active {
			t.Fatal("owner activation did not activate the school and membership")
		}
	})
}

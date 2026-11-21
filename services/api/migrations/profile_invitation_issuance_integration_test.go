package migrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testProfileInvitationIssuance(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	school, other := uuid.New(), uuid.New()
	admin, teacher, student, secondStudent, guardian, foreign := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	write := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	write(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'issue-fixture-a','Fictional A','ACTIVE'),
		($2,'issue-fixture-b','Fictional B','ACTIVE')`, school, other)
	write(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES
		($1,'issue-admin@example.test','unused','Fictional Admin','ACTIVE'),
		($2,'issue-teacher@example.test','unused','Fictional Teacher','ACTIVE')`, admin, teacher)
	write(`INSERT INTO platform_admins (user_id) VALUES ($1)`, admin)
	write(`INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($4,$1,$2,'SCHOOL_ADMIN'),($5,$1,$3,'TEACHER')`, school, admin, teacher, uuid.New(), uuid.New())
	write(`INSERT INTO students (id,tenant_id,admission_no,display_name,stage) VALUES
		($1,$2,'ISS-A','Fictional Student','PRIMARY'),($3,$2,'ISS-B','Fictional Second Student','PRIMARY')`, student, school, secondStudent)
	write(`INSERT INTO guardians (id,tenant_id,display_name,contact_email) VALUES
		($1,$2,'Fictional Guardian','guardian-issue@example.test'),
		($3,$4,'Fictional Foreign Guardian','foreign-issue@example.test')`, guardian, school, foreign, other)
	adminToken, teacherToken, csrf := uuid.NewString(), uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at) VALUES
		($1,$2,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour'),
		($3,$4,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour')`,
		admin, auth.HashToken(adminToken), teacher, auth.HashToken(teacherToken), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{
		Config: &config.Config{Environment: "test", PublicWebURL: "https://school.example.test",
			StaffInvitations: config.StaffInvitationLimits{PerActor: 2, PerTenant: 3, Window: time.Hour}},
		DB: &db.DB{Pool: pool},
	})
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path, token, header, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("profile invitation HTTP status %d, want %d", rec.Code, want)
		}
		return rec
	}
	const base = "/api/v1/tenants/issue-fixture-a/"
	studentPath := base + "students/" + student.String() + "/invitations"
	guardianPath := base + "guardians/" + guardian.String() + "/invitations"
	foreignPath := base + "guardians/" + foreign.String() + "/invitations"
	const payload = `{"email":"student-issue@example.test"}`
	send("POST", studentPath, "", "", payload, 401)
	send("POST", studentPath, adminToken, "", payload, 403)
	send("POST", studentPath, teacherToken, csrf, payload, 403)
	send("POST", foreignPath, adminToken, csrf, "{}", 404)
	send("POST", guardianPath, adminToken, csrf, `{"email":"override@example.test"}`, 422)
	send("POST", studentPath, adminToken, csrf, `{"email":"invalid"}`, 422)
	res := send("POST", studentPath, adminToken, csrf, payload, 201)
	if strings.Contains(res.Body.String(), "student-issue@example.test") || strings.Contains(res.Body.String(), "token") {
		t.Fatal("issuance response disclosed private invitation details")
	}
	send("POST", studentPath, adminToken, csrf, `{"email":"different-issue@example.test"}`, 409)
	send("POST", guardianPath, adminToken, csrf, "{}", 201)
	res = send("POST", base+"students/"+secondStudent.String()+"/invitations", adminToken, csrf, payload, 409)
	res = send("POST", base+"students/"+secondStudent.String()+"/invitations", adminToken, csrf, `{"email":"second-issue@example.test"}`, 429)
	if res.Header().Get("Retry-After") == "" {
		t.Fatal("quota response omitted retry guidance")
	}
	res = send("GET", "/api/v1/platform/mail-capture", adminToken, "", "", 200)
	var response struct {
		Data struct {
			Messages []struct{ To, Body string }
		}
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil || len(response.Data.Messages) != 2 {
		t.Fatal("expected mail capture for only the two successful requests")
	}
	for i, email := range []string{"student-issue@example.test", "guardian-issue@example.test"} {
		message := response.Data.Messages[i]
		if message.To != email || !strings.Contains(message.Body, "https://school.example.test/invite/") {
			t.Fatal("captured profile invitation has the wrong recipient or link")
		}
		_, link, _ := strings.Cut(message.Body, "https://school.example.test/invite/")
		token := strings.Fields(link)[0]
		body, err := json.Marshal(auth.AcceptInvitationRequest{
			Token: token, FullName: "Fictional Invitee", Password: "Sww-fixture-passphrase37!",
		})
		if err != nil {
			t.Fatal("encode fictional acceptance")
		}
		send("POST", "/api/v1/invitations/accept", "", "", string(body), 201)
		send("POST", "/api/v1/invitations/accept", "", "", string(body), 422)
	}
	var invitations, events int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM invitations WHERE tenant_id = $1 AND status = 'ACCEPTED'),
		(SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = 'tenant.profile_invitation.create')`, school).
		Scan(&invitations, &events); err != nil || invitations != 2 || events != 2 {
		t.Fatal("expected exactly two accepted invitations and two issuance events")
	}
}

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

func testSchoolOperatingMode(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, teacher uuid.UUID) {
	t.Helper()
	write := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	principal := uuid.New()
	write(`INSERT INTO users (id,email,password_hash,full_name,status)
		VALUES ($1,'mode-principal@example.test','unused','Fictional principal','ACTIVE')`, principal)
	membership := uuid.New()
	write(`INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'PRINCIPAL')`, membership, school, principal)
	adminToken, teacherToken, csrf := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for user, token := range map[uuid.UUID]string{principal: adminToken, teacher: teacherToken} {
		write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
			VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`, user, auth.HashToken(token), auth.HashToken(csrf))
	}
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	hit := func(method, path, token, header, body string, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("school mode HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	const mode = "/api/v1/tenants/lesson-fixture-a/school/mode"
	const schools = "/api/v1/my/schools"
	var dbTime time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbTime); err != nil {
		t.Fatal(err)
	}
	payload := func(mode, link string, start, end time.Time) string {
		t.Helper()
		data, err := json.Marshal(map[string]any{"mode": mode, "starts_at": start, "ends_at": end,
			"reason": "Fictional building maintenance", "help": "Contact the fictional school office", "schedule_url": link})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	remote := payload("REMOTE", "https://schedule.example.test/", dbTime.Add(-time.Hour), dbTime.Add(time.Hour))
	hit("PATCH", mode, "", "", remote, 401)
	hit("PATCH", mode, adminToken, "", remote, 403)
	hit("PATCH", mode, teacherToken, csrf, remote, 404)
	hit("PATCH", strings.Replace(mode, "lesson-fixture-a", "lesson-fixture-b", 1), adminToken, csrf, remote, 404)
	hit("PATCH", mode, adminToken, csrf, strings.Replace(remote, "https://schedule.example.test/", "https://user:pass@schedule.example.test/", 1), 422)
	hit("PATCH", mode, adminToken, csrf, payload("REMOTE", "https://schedule.example.test/", dbTime.Add(-2*time.Hour), dbTime.Add(-time.Hour)), 422)
	hit("PATCH", mode, adminToken, csrf, payload("REMOTE", "https://schedule.example.test/", dbTime.Add(time.Hour), dbTime.Add(2*time.Hour)), 200)
	if body := hit("GET", schools, teacherToken, "", "", 200); !strings.Contains(body, `"effective_mode":"ON_SITE"`) {
		t.Fatal("scheduled future mode must not be effective yet")
	}
	hit("PATCH", mode, adminToken, csrf, remote, 200)
	if body := hit("GET", schools, teacherToken, "", "", 200); !strings.Contains(body, `"effective_mode":"REMOTE"`) ||
		!strings.Contains(body, "Fictional building maintenance") {
		t.Fatal("active member did not see remote mode")
	}
	hit("PATCH", mode, adminToken, csrf, payload("HYBRID", "https://schedule.example.test/", dbTime.Add(-time.Hour), dbTime.Add(time.Hour)), 200)
	if body := hit("GET", schools, teacherToken, "", "", 200); !strings.Contains(body, `"effective_mode":"HYBRID"`) {
		t.Fatal("hybrid mode missing")
	}
	write(`UPDATE memberships SET status='SUSPENDED' WHERE id=$1`, membership)
	hit("PATCH", mode, adminToken, csrf, `{"mode":"ON_SITE"}`, 404)
	write(`UPDATE memberships SET status='ACTIVE' WHERE id=$1`, membership)
	write(`UPDATE tenants SET mode_starts_at=clock_timestamp()-interval '2 hours',mode_ends_at=clock_timestamp()-interval '1 hour' WHERE id=$1`, school)
	if body := hit("GET", schools, teacherToken, "", "", 200); !strings.Contains(body, `"effective_mode":"ON_SITE"`) {
		t.Fatal("expired schedule must fall back to on site")
	}
	hit("PATCH", mode, adminToken, csrf, `{"mode":"ON_SITE"}`, 200)
	if body := hit("GET", schools, teacherToken, "", "", 200); strings.Contains(body, "operating_schedule") {
		t.Fatal("restoring on-site mode must clear the schedule")
	}
	var termID uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT id FROM academic_terms WHERE tenant_id=$1 AND sequence=1`, school).Scan(&termID); err != nil {
		t.Fatal(err)
	}
	schemes := "/api/v1/tenants/lesson-fixture-a/terms/" + termID.String() + "/assessment-schemes"
	draft := `{"stage":"JUNIOR_SECONDARY","components":[{"name":"Fictional coursework","max_score":"20","weight":"30.25"}]}`
	hit("GET", schemes, teacherToken, "", "", 403)
	hit("POST", schemes, adminToken, "", draft, 403)
	hit("POST", strings.Replace(schemes, "lesson-fixture-a", "lesson-fixture-b", 1), adminToken, csrf, draft, 404)
	var created struct{ Data struct{ ID uuid.UUID } }
	if err = json.Unmarshal([]byte(hit("POST", schemes, adminToken, csrf, draft, 201)), &created); err != nil || created.Data.ID == uuid.Nil {
		t.Fatal("assessment draft identifier missing")
	}
	hit("POST", schemes, adminToken, csrf, draft, 409)
	detail := schemes + "/" + created.Data.ID.String()
	publish := detail + "/publish"
	hit("POST", publish, adminToken, csrf, `{"revision":1}`, 422)
	hit("PUT", detail, adminToken, csrf, `{"revision":0,"components":[{"name":"Stale fixture","max_score":"10","weight":"100"}]}`, 409)
	hit("PUT", detail, adminToken, csrf, `{"revision":1,"components":[{"name":"Fictional overlap","max_score":"20","weight":"100"}],
		"grade_bands":[{"label":"A","min_bp":7000,"max_bp":10000},{"label":"B","min_bp":0,"max_bp":7000}]}`, 422)
	hit("PUT", detail, adminToken, csrf, `{"revision":1,"grade_bands":[{"label":"A","min_bp":7000,"max_bp":10000},{"label":"B","min_bp":0,"max_bp":6999}],"components":[
		{"name":"Fictional coursework","max_score":"20","weight":"30.25"},
		{"name":"Fictional examination","max_score":"50","weight":"69.75"}
	]}`, 200)
	hit("POST", publish, adminToken, csrf, `{"revision":1}`, 409)
	hit("POST", publish, adminToken, csrf, `{"revision":5}`, 200)
	hit("PUT", detail, adminToken, csrf, `{"revision":5,"components":[{"name":"Changed fixture","max_score":"10","weight":"100"}]}`, 409)
	if body := hit("GET", schemes, adminToken, "", "", 200); !strings.Contains(body, "Fictional examination") || !strings.Contains(body, `"max_bp":6999`) || !strings.Contains(body, `"status":"PUBLISHED"`) {
		t.Fatal("published assessment scheme and its components are unavailable")
	}
	var audits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND action='tenant.operating_mode.update'
		AND NOT (metadata_redacted ? 'reason')`, school).Scan(&audits); err != nil || audits != 4 {
		t.Fatal("only successful updates should write redacted mode audits")
	}
}

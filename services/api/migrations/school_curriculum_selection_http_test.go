package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testSchoolCurriculumHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, session, manager, eligible, withdrawn uuid.UUID) {
	write := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	teacher := uuid.New()
	write(`INSERT INTO users (id,email,password_hash,full_name,status)
		VALUES ($1,'selection-teacher@example.test','unused','Fictional teacher','ACTIVE')`, teacher)
	write(`INSERT INTO memberships (id,tenant_id,user_id,role)
		VALUES ($1,$4,$5,'BURSAR'),($2,$4,$5,'PRINCIPAL'),($3,$4,$6,'TEACHER')`,
		uuid.New(), uuid.New(), uuid.New(), school, manager, teacher)
	managerToken, teacherToken, csrf := uuid.NewString(), uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at) VALUES
		($1,$2,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour'),
		($3,$4,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour')`,
		manager, auth.HashToken(managerToken), teacher, auth.HashToken(teacherToken), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token, header, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	check := func(method, path, token, header, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		rec := request(method, path, token, header, body)
		if rec.Code != status {
			t.Fatalf("school selection HTTP status %d, want %d", rec.Code, status)
		}
		return rec
	}
	sessionPath := "/academic-sessions/" + session.String() + "/curriculum"
	path := "/api/v1/tenants/choice-fixture-a" + sessionPath
	body := func(edition uuid.UUID, revision int) string {
		return fmt.Sprintf(`{"stage":"PRIMARY","curriculum_role":"PRIMARY","framework_version_id":%q,"expected_revision":%d}`, edition, revision)
	}
	rec := check("GET", path, managerToken, "", "", 200)
	if rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), withdrawn.String()) ||
		!strings.Contains(rec.Body.String(), `"framework_status":"WITHDRAWN"`) {
		t.Fatal("current withdrawn selection must remain explicitly visible")
	}
	check("GET", path, "", "", "", 401)
	check("GET", path, teacherToken, "", "", 404)
	check("GET", "/api/v1/tenants/choice-fixture-b"+sessionPath, managerToken, "", "", 404)
	check("POST", path, managerToken, "", body(eligible, 2), 403)
	check("POST", path, teacherToken, csrf, body(eligible, 2), 404)
	check("POST", "/api/v1/tenants/choice-fixture-b"+sessionPath, managerToken, csrf, body(eligible, 2), 404)
	check("POST", path, managerToken, csrf, `{"stage":"PRIMARY","curriculum_role":"PRIMARY"}`, 422)
	check("POST", path, managerToken, csrf, body(withdrawn, 2), 422)
	check("POST", path, managerToken, csrf, body(eligible, 1), 409)

	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { responses <- request("POST", path, managerToken, csrf, body(eligible, 2)) }()
	}
	var result struct {
		Data struct {
			ID       uuid.UUID
			Revision int
		}
	}
	created, conflicts := 0, 0
	for range 2 {
		rec := <-responses
		switch rec.Code {
		case 201:
			created++
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal("invalid selection response")
			}
		case 409:
			conflicts++
		}
	}
	if created != 1 || conflicts != 1 || result.Data.Revision != 3 || result.Data.ID == uuid.Nil {
		t.Fatal("exactly one concurrent write should append revision three")
	}
	var rows, audits int
	err = pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM school_curriculum_selections WHERE tenant_id=$1 AND session_id=$2 AND stage='PRIMARY' AND curriculum_role='PRIMARY'),
		(SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND actor_id=$3 AND action='curriculum.selection.create' AND subject_id=$4)`,
		school, session, manager, result.Data.ID.String()).Scan(&rows, &audits)
	if err != nil || rows != 3 || audits != 1 {
		t.Fatal("expected one appended revision with one audit event")
	}
	write(`UPDATE academic_sessions SET status='ARCHIVED' WHERE id=$1`, session)
	check("POST", path, managerToken, csrf, body(eligible, 3), 409)
	rec = check("GET", path, managerToken, "", "", 200)
	if !strings.Contains(rec.Body.String(), `"session_status":"ARCHIVED"`) {
		t.Fatal("archived session should remain readable to authorized managers")
	}
	write(`UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=$2 AND role='PRINCIPAL'`, school, manager)
	check("GET", path, managerToken, "", "", 404)
}

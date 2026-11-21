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
	"github.com/school-without-walls/api/internal/modules/auth"
)

func teacherLessonReview(t *testing.T, ctx context.Context, pool *pgxpool.Pool, router http.Handler, school, actor uuid.UUID, teacher, csrf, payload string) {
	t.Helper()
	reviewer, reviewerToken := uuid.New(), uuid.NewString()
	run := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	run(`UPDATE tenants SET feature_flags='{"lesson_review_required":true}' WHERE id=$1`, school)
	run(`INSERT INTO users (id,email,password_hash,full_name,status)
	VALUES ($1,'lesson-reviewer@example.test','unused','Fictional reviewer','ACTIVE')`, reviewer)
	run(`INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$3,$4,'PRINCIPAL'),($2,$3,$5,'PRINCIPAL')`,
		uuid.New(), uuid.New(), school, reviewer, actor)
	run(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
	VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`, reviewer, auth.HashToken(reviewerToken), auth.HashToken(csrf))
	hit := func(method, path, token, header, body string, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("review HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	const base = "/api/v1/tenants/lesson-fixture-a"
	var result struct{ Data struct{ ID uuid.UUID } }
	out := hit("POST", base+"/teaching/lessons", teacher, csrf, payload, 201)
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Data.ID == uuid.Nil {
		t.Fatal("draft ID missing")
	}
	lesson := base + "/teaching/lessons/" + result.Data.ID.String()
	reviews := base + "/lesson-reviews"
	approval := reviews + "/" + result.Data.ID.String() + "/approve"
	hit("POST", lesson+"/publish", teacher, csrf, "", 409)
	hit("POST", lesson+"/submit", teacher, csrf, "", 200)
	if body := hit("GET", reviews, reviewerToken, "", "", 200); !strings.Contains(body, result.Data.ID.String()) {
		t.Fatal("pending review missing")
	}
	if body := hit("GET", reviews, teacher, "", "", 200); strings.Contains(body, result.Data.ID.String()) {
		t.Fatal("author saw own draft in review queue")
	}
	revision := func(n int) string { return fmt.Sprintf(`{"revision":%d}`, n) }
	hit("POST", approval, teacher, csrf, revision(1), 404)
	hit("POST", strings.Replace(approval, "lesson-fixture-a", "lesson-fixture-b", 1), reviewerToken, csrf, revision(1), 404)
	hit("POST", approval, reviewerToken, "", revision(1), 403)
	hit("POST", approval, reviewerToken, csrf, revision(1), 200)
	hit("PATCH", lesson, teacher, csrf, `{"title":"New revision","content":"Revised text"}`, 200)
	hit("POST", lesson+"/publish", teacher, csrf, "", 409)
	hit("POST", approval, reviewerToken, csrf, revision(1), 409)
	hit("POST", lesson+"/submit", teacher, csrf, "", 200)
	hit("POST", approval, reviewerToken, csrf, revision(2), 200)
	hit("POST", lesson+"/publish", teacher, csrf, "", 200)
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_id=$2 AND action='learning.lesson.approve'`,
		school, result.Data.ID.String()).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatal("expected two approvals audited across revisions")
	}
}

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

func testStudentLessonHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, session, term, class, lesson, draft uuid.UUID) {
	t.Helper()
	write := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	user, student, enrollment := uuid.New(), uuid.New(), uuid.New()
	write(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES ($1,'lesson-learner@example.test','unused','Fictional learner','ACTIVE')`, user)
	write(`INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'STUDENT')`, uuid.New(), school, user)
	write(`INSERT INTO students (id,tenant_id,user_id,admission_no,display_name,stage)
		VALUES ($1,$2,$3,'LESSON-1','Fictional learner','PRIMARY')`, student, school, user)
	token, csrf := uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
		VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour')`, user, auth.HashToken(token), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	get := func(path, credential string, expected int) string {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != expected {
			t.Fatalf("lesson status %d, want %d", rec.Code, expected)
		}
		return rec.Body.String()
	}
	const endpoint = "/api/v1/tenants/lesson-fixture-a/learning/lessons"
	detail := endpoint + "/" + lesson.String()
	progress := func(path, credential, header, body string, expected int) string {
		t.Helper()
		req := httptest.NewRequest("PUT", path, strings.NewReader(body))
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != expected {
			t.Fatalf("progress status %d, want %d", rec.Code, expected)
		}
		return rec.Body.String()
	}
	progressURL := detail + "/progress"
	const start = `{"state":"IN_PROGRESS"}`
	const complete = `{"state":"COMPLETED"}`
	progress(progressURL, "", "", start, 401)
	progress(progressURL, token, csrf, start, 404)
	get(detail, "", 401)
	get(detail, token, 404)
	write(`INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id) VALUES ($1,$2,$3,$4,$5,$6)`,
		enrollment, school, student, session, term, class)
	if body := get(endpoint, token, 200); !strings.Contains(body, lesson.String()) || strings.Contains(body, draft.String()) || strings.Contains(body, "Count fictional objects.") {
		t.Fatal("feed must expose only published summaries")
	}
	if body := get(detail, token, 200); !strings.Contains(body, "Count fictional objects.") {
		t.Fatal("eligible student should see lesson text")
	}
	get(endpoint+"/"+draft.String(), token, 404)
	get(strings.Replace(detail, "lesson-fixture-a", "lesson-fixture-b", 1), token, 404)
	if body := get(detail, token, 200); !strings.Contains(body, `"progress_state":"NOT_STARTED"`) {
		t.Fatal("unstarted progress must appear on the lesson")
	}
	progress(progressURL, token, "", start, 403)
	progress(endpoint+"/"+draft.String()+"/progress", token, csrf, start, 404)
	progress(strings.Replace(progressURL, "lesson-fixture-a", "lesson-fixture-b", 1), token, csrf, start, 404)
	progress(progressURL, token, csrf, `{"state":"COMPLETED","student_id":"another-student"}`, 422)
	progress(progressURL, token, csrf, `{"state":"NOT_STARTED"}`, 422)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lesson_progress WHERE tenant_id=$1`, school).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied requests wrote progress")
	}
	type progressRecord struct {
		State       string
		StartedAt   time.Time  `json:"started_at"`
		CompletedAt *time.Time `json:"completed_at"`
	}
	parse := func(body string) progressRecord {
		t.Helper()
		var result struct{ Data progressRecord }
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatal("invalid progress response")
		}
		return result.Data
	}
	first := parse(progress(progressURL, token, csrf, start, 200))
	repeat := parse(progress(progressURL, token, csrf, start, 200))
	if first.State != "IN_PROGRESS" || first.StartedAt.IsZero() || first.CompletedAt != nil || !first.StartedAt.Equal(repeat.StartedAt) || repeat.CompletedAt != nil {
		t.Fatal("repeat start must preserve the initial timestamp and in-progress state")
	}
	done := parse(progress(progressURL, token, csrf, complete, 200))
	replay := parse(progress(progressURL, token, csrf, start, 200))
	if done.State != "COMPLETED" || done.CompletedAt == nil || done.CompletedAt.Before(first.StartedAt) ||
		replay.State != "COMPLETED" || replay.CompletedAt == nil || !replay.CompletedAt.Equal(*done.CompletedAt) ||
		!replay.StartedAt.Equal(first.StartedAt) {
		t.Fatal("completion must be monotonic and preserve the first timestamps")
	}
	if body := get(endpoint, token, 200); !strings.Contains(body, `"progress_state":"COMPLETED"`) {
		t.Fatal("student feed must include saved progress")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lesson_progress WHERE tenant_id=$1 AND lesson_id=$2 AND student_id=$3`,
		school, lesson, student).Scan(&count); err != nil || count != 1 {
		t.Fatal("expected exactly one progress record for the authenticated learner")
	}
	write(`UPDATE enrollments SET status='ENDED',ended_at=clock_timestamp() WHERE id=$1`, enrollment)
	progress(progressURL, token, csrf, complete, 404)
	get(detail, token, 404)
	if body := get(endpoint, token, 200); strings.Contains(body, lesson.String()) {
		t.Fatal("ended enrollment disclosed a lesson")
	}
}

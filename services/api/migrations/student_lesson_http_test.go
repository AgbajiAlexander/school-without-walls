package migrations

import (
	"context"
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
	token := uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
		VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour')`, user, auth.HashToken(token), auth.HashToken(uuid.NewString()))
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
	write(`UPDATE enrollments SET status='ENDED',ended_at=clock_timestamp() WHERE id=$1`, enrollment)
	get(detail, token, 404)
	if body := get(endpoint, token, 200); strings.Contains(body, lesson.String()) {
		t.Fatal("ended enrollment disclosed a lesson")
	}
}

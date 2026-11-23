package migrations

import (
	"context"
	"encoding/json"
	"fmt"
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
	assignments := detail + "/assignments"
	write(`INSERT INTO assignments (id,tenant_id,lesson_id,title,instructions,due_at,max_score,submission_type)
		VALUES ($1,$2,$3,'Private homework draft','Keep private.',clock_timestamp()+interval '1 day',10,'TEXT')`,
		uuid.New(), school, lesson)
	get(assignments, "", 401)
	if body := get(assignments, token, 200); !strings.Contains(body, "Counting practice") || strings.Contains(body, "Private homework draft") {
		t.Fatal("student assignment list must expose released assignments only")
	}
	if body := get(strings.Replace(assignments, "lesson-fixture-a", "lesson-fixture-b", 1), token, 200); strings.Contains(body, "Counting practice") {
		t.Fatal("foreign school disclosed an assignment")
	}
	createHomework := func(permit bool, kind string) uuid.UUID {
		t.Helper()
		id := uuid.New()
		write(`INSERT INTO assignments (id,tenant_id,lesson_id,title,instructions,due_at,max_score,submission_type,allow_resubmission)
			VALUES ($1,$2,$3,'Submission fixture','Write an answer.',clock_timestamp()+interval '1 day',10,$4,$5)`,
			id, school, lesson, kind, permit)
		write(`UPDATE assignments SET status='OPEN' WHERE id=$1`, id)
		return id
	}
	one := createHomework(false, "TEXT")
	multiple := createHomework(true, "TEXT")
	file := createHomework(false, "FILE")
	send := func(path, header, body string, want int) string {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("submission status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	submission := func(id uuid.UUID) string { return assignments + "/" + id.String() + "/submissions" }
	body := func(key uuid.UUID, answer string) string {
		return fmt.Sprintf(`{"request_id":%q,"content":%q}`, key, answer)
	}
	key := uuid.New()
	original := body(key, "A fictional private answer")
	send(submission(one), "", original, 403)
	send(strings.Replace(submission(one), "lesson-fixture-a", "lesson-fixture-b", 1), csrf, original, 404)
	send(submission(one), csrf, `{"student_id":"wrong","content":"forged"}`, 422)
	send(submission(file), csrf, original, 409)
	expiring := uuid.New()
	write(`INSERT INTO assignments (id,tenant_id,lesson_id,title,instructions,due_at,max_score,submission_type)
		VALUES ($1,$2,$3,'Deadline fixture','Write an answer.',clock_timestamp()+interval '3 seconds',10,'TEXT')`,
		expiring, school, lesson)
	write(`UPDATE assignments SET status='OPEN' WHERE id=$1`, expiring)
	write(`SELECT pg_sleep_until(due_at) FROM assignments WHERE id=$1`, expiring)
	send(submission(expiring), csrf, original, 409)
	receiptData := func(reply string) string {
		t.Helper()
		var decoded struct{ Data json.RawMessage }
		if err := json.Unmarshal([]byte(reply), &decoded); err != nil || len(decoded.Data) == 0 {
			t.Fatal("invalid submission receipt")
		}
		return string(decoded.Data)
	}
	firstAnswer := receiptData(send(submission(one), csrf, original, 201))
	if replay := receiptData(send(submission(one), csrf, original, 200)); replay != firstAnswer {
		t.Fatal("retry must return the original receipt")
	}
	send(submission(one), csrf, body(key, "Changed answer"), 409)
	send(submission(one), csrf, body(uuid.New(), "Second answer"), 409)
	send(submission(multiple), csrf, original, 201)
	if reply := send(submission(multiple), csrf, body(uuid.New(), "Revised answer"), 201); !strings.Contains(reply, `"revision":2`) {
		t.Fatal("enabled resubmission must create a new revision")
	}
	write(`UPDATE assignments SET status='CLOSED' WHERE id=$1`, multiple)
	send(submission(multiple), csrf, body(uuid.New(), "Late answer"), 409)
	if listed := get(assignments, token, 200); !strings.Contains(listed, `"submission":{"revision":2`) || strings.Contains(listed, "private answer") {
		t.Fatal("student assignment list must return receipt metadata without answer text")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assignment_submissions WHERE tenant_id=$1 AND student_id=$2`, school, student).Scan(&count); err != nil || count != 3 {
		t.Fatal("only accepted revisions may create submission records")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND actor_id=$2 AND action='learning.assignment.submit'`, school, user).Scan(&count); err != nil || count != 3 {
		t.Fatal("only accepted revisions may produce submission audits")
	}
	write(`UPDATE enrollments SET status='ENDED',ended_at=clock_timestamp() WHERE id=$1`, enrollment)
	send(submission(one), csrf, original, 404)
	if body := get(assignments, token, 200); strings.Contains(body, "Counting practice") {
		t.Fatal("ended enrollment disclosed an assignment")
	}
	progress(progressURL, token, csrf, complete, 404)
	get(detail, token, 404)
	if body := get(endpoint, token, 200); strings.Contains(body, lesson.String()) {
		t.Fatal("ended enrollment disclosed a lesson")
	}
}

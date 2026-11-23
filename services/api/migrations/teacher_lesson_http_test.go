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

func testTeacherLessonHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, actor, membership, offering, objective, staleObjective uuid.UUID) {
	t.Helper()
	write := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	token, csrf := uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
  VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`, actor, auth.HashToken(token), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path, header, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("teacher lesson HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.Bytes()
	}
	const path = "/api/v1/tenants/lesson-fixture-a/teaching/lessons"
	payload := func(id uuid.UUID) string {
		return fmt.Sprintf(`{"offering_id":%q,"primary_objective_id":%q,"title":"Fictional lesson","content":"Original lesson text"}`, offering, id)
	}
	offerings := strings.TrimSuffix(path, "lessons") + "offerings"
	objectives := offerings + "/" + offering.String() + "/objectives"
	if body := send("GET", offerings, "", "", 200); strings.Contains(string(body), offering.String()) {
		t.Fatal("unassigned offering was disclosed")
	}
	send("POST", path, csrf, payload(objective), 404)
	assignment := uuid.New()
	write(`INSERT INTO teacher_assignments (id,tenant_id,offering_id,teacher_membership_id) VALUES ($1,$2,$3,$4)`, assignment, school, offering, membership)
	if body := send("GET", offerings, "", "", 200); !strings.Contains(string(body), offering.String()) {
		t.Fatal("assigned offering was missing")
	}
	if body := send("GET", objectives, "", "", 200); !strings.Contains(string(body), objective.String()) || strings.Contains(string(body), staleObjective.String()) {
		t.Fatal("objective list must use the offering's current primary selection")
	}
	send("POST", path, "", payload(objective), 403)
	send("POST", strings.Replace(path, "lesson-fixture-a", "lesson-fixture-b", 1), csrf, payload(objective), 404)
	send("POST", path, csrf, payload(staleObjective), 422)
	var result struct{ Data struct{ ID uuid.UUID } }
	if err := json.Unmarshal(send("POST", path, csrf, payload(objective), 201), &result); err != nil || result.Data.ID == uuid.Nil {
		t.Fatal("draft ID missing")
	}
	detail := path + "/" + result.Data.ID.String()
	if body := send("GET", path, "", "", 200); !strings.Contains(string(body), result.Data.ID.String()) || strings.Contains(string(body), "Original lesson text") {
		t.Fatal("lesson list must include draft summaries without content")
	}
	if body := send("GET", detail, "", "", 200); !strings.Contains(string(body), "Original lesson text") {
		t.Fatal("assigned author must see draft content")
	}
	send("GET", strings.Replace(detail, "lesson-fixture-a", "lesson-fixture-b", 1), "", "", 404)
	send("PATCH", detail, csrf, `{"title":"Edited lesson","content":"Edited text"}`, 200)
	homework := detail + "/assignments"
	draft := fmt.Sprintf(`{"title":"Counting draft","instructions":"Count five fictional objects.","due_at":%q,"max_score":10,"submission_type":"TEXT"}`,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339))
	send("POST", homework, "", draft, 403)
	send("POST", strings.Replace(homework, "lesson-fixture-a", "lesson-fixture-b", 1), csrf, draft, 404)
	send("POST", homework, csrf, strings.Replace(draft, `"max_score":10`, `"max_score":0`, 1), 422)
	var saved struct{ Data struct{ ID uuid.UUID } }
	if err := json.Unmarshal(send("POST", homework, csrf, draft, 201), &saved); err != nil || saved.Data.ID == uuid.Nil {
		t.Fatal("assignment ID missing")
	}
	item := homework + "/" + saved.Data.ID.String()
	send("POST", item+"/open", csrf, "", 409)
	edited := strings.Replace(draft, "Counting draft", "Edited counting draft", 1)
	send("PATCH", item, csrf, edited, 200)
	if body := send("GET", homework, "", "", 200); !strings.Contains(string(body), "Edited counting draft") {
		t.Fatal("teacher assignment list did not return the saved draft")
	}
	write(`UPDATE teacher_assignments SET status='INACTIVE' WHERE id=$1`, assignment)
	send("GET", detail, "", "", 404)
	send("POST", detail+"/publish", csrf, "", 404)
	send("PATCH", item, csrf, edited, 404)
	write(`INSERT INTO teacher_assignments (id,tenant_id,offering_id,teacher_membership_id) VALUES ($1,$2,$3,$4)`, uuid.New(), school, offering, membership)
	send("POST", detail+"/publish", csrf, "", 200)
	send("POST", item+"/open", csrf, "", 200)
	send("POST", item+"/open", csrf, "", 409)
	send("PATCH", item, csrf, draft, 409)
	send("POST", item+"/close", csrf, "", 200)
	submission := uuid.New()
	write(`INSERT INTO assignment_submissions (id,tenant_id,assignment_id,student_id,request_id,revision,content,submitted_at)
		SELECT $1,tenant_id,$2,id,$3,1,'Fictional answer',clock_timestamp() FROM students WHERE tenant_id=$4`,
		submission, saved.Data.ID, uuid.New(), school)
	queue := item + "/submissions"
	feedback := queue + "/" + submission.String() + "/feedback"
	if body := send("GET", queue, "", "", 200); !strings.Contains(string(body), "Fictional answer") {
		t.Fatal("teacher cannot read scoped submission")
	}
	send("PUT", feedback, "", `{"score":7,"comment":"Good work","version":0}`, 403)
	send("PUT", strings.Replace(feedback, "lesson-fixture-a", "lesson-fixture-b", 1), csrf, `{"score":7,"comment":"Good work","version":0}`, 404)
	send("PUT", feedback, csrf, `{"score":11,"comment":"Out of range","version":0}`, 422)
	send("PUT", feedback, csrf, `{"score":7,"comment":"Good work","version":0}`, 200)
	send("PUT", feedback, csrf, `{"score":10,"comment":"Stale feedback","version":0}`, 409)
	send("PUT", feedback, csrf, `{"score":8,"comment":"Rechecked","version":1}`, 200)
	if body := send("GET", queue, "", "", 200); !strings.Contains(string(body), `"score":8`) {
		t.Fatal("teacher list did not return the current score")
	}
	var feedbackAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND action='learning.assignment.feedback'
		AND subject_id=$2 AND NOT (metadata_redacted ? 'comment')`, school, submission.String()).Scan(&feedbackAudits); err != nil || feedbackAudits != 2 {
		t.Fatal("feedback revisions must be audited without the comment")
	}
	var assignmentAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_type='assignment' AND subject_id=$2`,
		school, saved.Data.ID.String()).Scan(&assignmentAudits); err != nil || assignmentAudits != 4 {
		t.Fatal("exactly the successful assignment lifecycle mutations must be audited")
	}
	send("PATCH", detail, csrf, `{"title":"Changed","content":"Changed"}`, 409)
	send("POST", detail+"/publish", csrf, "", 409)
	send("POST", detail+"/archive", csrf, "", 200)
	teacherLessonReview(t, ctx, pool, router, school, actor, token, csrf, payload(objective))
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND actor_id=$2 AND subject_type='lesson' AND subject_id=$3`, school, actor, result.Data.ID.String()).Scan(&audits); err != nil || audits != 4 {
		t.Fatal("expected exactly one audit per successful mutation")
	}
}

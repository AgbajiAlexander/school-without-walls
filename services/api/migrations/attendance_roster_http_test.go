package migrations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testAttendanceRoster(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, foreign, teacher, member, session, term, class, subject uuid.UUID) {
	t.Helper()
	exec := func(want, sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		if want == "" && err == nil {
			return
		}
		var pgErr *pgconn.PgError
		if want == "" || !errors.As(err, &pgErr) || pgErr.Code != want {
			t.Fatalf("attendance fixture SQLSTATE: wanted %q, got %v", want, err)
		}
	}
	student, firstArm, secondArm, oldOffering, newOffering, oldPlacement, newPlacement := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("", `INSERT INTO students (id,tenant_id,admission_no,display_name,stage) VALUES ($1,$2,'ROLL-1','Fictional transfer learner','PRIMARY')`,
		student, school)
	exec("", `INSERT INTO class_arms (id,tenant_id,class_id,code,name)
		VALUES ($1,$3,$4,'ATT-A','Fixture A'),($2,$3,$4,'ATT-B','Fixture B')`, firstArm, secondArm, school, class)
	exec("", `INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id,class_arm_id)
		VALUES ($1,$3,$4,$5,$6,$7),($2,$3,$4,$5,$6,$8)`, oldOffering, newOffering, school, subject, term, class, firstArm, secondArm)
	for _, offering := range []uuid.UUID{oldOffering, newOffering} {
		exec("", `INSERT INTO teacher_assignments (id,tenant_id,offering_id,teacher_membership_id) VALUES ($1,$2,$3,$4)`,
			uuid.New(), school, offering, member)
	}
	var date string
	if err := pool.QueryRow(ctx, `SELECT ((statement_timestamp() AT TIME ZONE timezone)::date-1)::text FROM tenants WHERE id=$1`, school).Scan(&date); err != nil {
		t.Fatal(err)
	}
	exec("", `INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id,class_arm_id,status,created_at,ended_at)
		SELECT e.id,$3,$4,$5,$6,$7,e.arm,'ENDED',($10::date+e.start_at) AT TIME ZONE t.timezone,($10::date+e.end_at) AT TIME ZONE t.timezone
		FROM tenants t CROSS JOIN (VALUES ($1::uuid,$8::uuid,'08:00'::time,'12:00'::time),($2::uuid,$9::uuid,'12:00'::time,'16:00'::time))
			e(id,arm,start_at,end_at) WHERE t.id=$3`, oldPlacement, newPlacement, school, student, session, term, class, firstArm, secondArm, date)
	const mark = `INSERT INTO attendance_records (id,tenant_id,enrollment_id,student_id,session_id,term_id,class_id,on_date,mode,status,recorded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::date,$9,$10,$11)`
	exec("23503", mark, uuid.New(), foreign, newPlacement, student, session, term, class, date, "REMOTE", "LATE", member)
	exec("23514", mark, uuid.New(), school, newPlacement, student, session, term, class, date, "HYBRID", "LATE", member)
	exec("", mark, uuid.New(), school, newPlacement, student, session, term, class, date, "REMOTE", "LATE", member)
	exec("23505", mark, uuid.New(), school, newPlacement, student, session, term, class, date, "ON_SITE", "PRESENT", member)

	token, csrf := uuid.NewString(), uuid.NewString()
	exec("", `INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
		VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`,
		teacher, auth.HashToken(token), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	get := func(slug string, offering uuid.UUID, date, credential string, want int) string {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/tenants/"+slug+"/teaching/offerings/"+offering.String()+"/attendance?date="+date, nil)
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("attendance roster HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	const slug = "lesson-fixture-a"
	get(slug, newOffering, date, "", 401)
	get(slug, newOffering, "invalid", token, 422)
	if body := get(slug, newOffering, date, token, 200); !strings.Contains(body, "Fictional transfer learner") || !strings.Contains(body, `"mode":"REMOTE"`) || !strings.Contains(body, `"status":"LATE"`) {
		t.Fatal("the final placement on the date must expose its persisted mark")
	}
	for _, body := range []string{
		get(slug, oldOffering, date, token, 200),
		get("lesson-fixture-b", newOffering, date, token, 200),
		get(slug, newOffering, "2099-01-01", token, 200),
	} {
		if strings.Contains(body, "Fictional transfer learner") {
			t.Fatal("ineligible attendance roster disclosed a learner")
		}
	}
	var version time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM attendance_records WHERE tenant_id=$1 AND student_id=$2`, school, student).Scan(&version); err != nil {
		t.Fatal(err)
	}
	send := func(slug string, offering uuid.UUID, header, reason string, expected time.Time, status string, want int) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{"date": date, "marks": []map[string]any{{
			"student_id": student, "mode": "REMOTE", "status": status, "reason": reason, "expected_updated_at": expected,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("PUT", "/api/v1/tenants/"+slug+"/teaching/offerings/"+offering.String()+"/attendance", strings.NewReader(string(body)))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("attendance save HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	send(slug, newOffering, "", "Fixture correction", version, "PRESENT", 403)
	send("lesson-fixture-b", newOffering, csrf, "Fixture correction", version, "PRESENT", 404)
	send(slug, oldOffering, csrf, "Fixture correction", version, "PRESENT", 404)
	send(slug, newOffering, csrf, "", version, "PRESENT", 422)
	if body := send(slug, newOffering, csrf, "Fixture correction", version, "PRESENT", 200); !strings.Contains(body, `"changed":1`) {
		t.Fatal("correction was not recorded")
	}
	if body := send(slug, newOffering, csrf, "Fixture correction", version, "PRESENT", 200); !strings.Contains(body, `"changed":0`) {
		t.Fatal("identical replay created a change")
	}
	send(slug, newOffering, csrf, "Stale correction", version, "ABSENT", 409)
	if body := get(slug, newOffering, date, token, 200); !strings.Contains(body, `"status":"PRESENT"`) {
		t.Fatal("corrected roster mark missing")
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND action='attendance.mark.save'
		AND metadata_redacted->>'previous_status'='LATE' AND metadata_redacted->>'status'='PRESENT'
		AND metadata_redacted->>'reason'='Fixture correction'`, school).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("expected exactly one correction audit with both values and the reason")
	}
	parent, guardian, parentMembership := uuid.New(), uuid.New(), uuid.New()
	parentToken := uuid.NewString()
	exec("", `INSERT INTO users (id,email,password_hash,full_name,status)
		VALUES ($1,'attendance-parent@example.test','unused','Fictional Parent','ACTIVE')`, parent)
	exec("", `INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'PARENT')`, parentMembership, school, parent)
	exec("", `INSERT INTO guardians (id,tenant_id,user_id,display_name) VALUES ($1,$2,$3,'Fictional Parent')`, guardian, school, parent)
	exec("", `INSERT INTO student_guardians (tenant_id,student_id,guardian_id,relationship) VALUES ($1,$2,$3,'Parent')`, school, student, guardian)
	exec("", `INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
		VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`, parent, auth.HashToken(parentToken), auth.HashToken(csrf))
	family := func(path, credential string, want int) string {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("family attendance status %d, want %d", rec.Code, want)
		}
		if credential != "" && rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("family response must not be stored")
		}
		return rec.Body.String()
	}
	const children = "/api/v1/tenants/lesson-fixture-a/family/attendance"
	child := children + "/" + student.String()
	family(child, "", 401)
	family(child, token, 404)
	family(strings.Replace(child, "lesson-fixture-a", "lesson-fixture-b", 1), parentToken, 404)
	var unlinked uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id<>$2 LIMIT 1`, school, student).Scan(&unlinked); err != nil {
		t.Fatal(err)
	}
	family(children+"/"+unlinked.String(), parentToken, 404)
	if body := family(children, parentToken, 200); !strings.Contains(body, student.String()) || strings.Contains(body, unlinked.String()) {
		t.Fatal("family list must contain only linked children")
	}
	if body := family(child, parentToken, 200); !strings.Contains(body, `"status":"PRESENT"`) || !strings.Contains(body, `"mode":"REMOTE"`) ||
		strings.Contains(body, "Fixture correction") || strings.Contains(body, "recorded_by") {
		t.Fatal("family view must show the corrected mark without staff audit details")
	}
	exec("", `UPDATE memberships SET status='SUSPENDED' WHERE id=$1`, parentMembership)
	family(child, parentToken, 404)
	exec("", `UPDATE memberships SET status='ACTIVE' WHERE id=$1`, parentMembership)
	exec("", `UPDATE student_guardians SET active=false WHERE tenant_id=$1 AND student_id=$2 AND guardian_id=$3`, school, student, guardian)
	family(child, parentToken, 404)
	if body := family(children, parentToken, 200); strings.Contains(body, student.String()) {
		t.Fatal("unlink must remove the learner from the family list")
	}
	unmarked := uuid.New()
	exec("", `INSERT INTO students (id,tenant_id,admission_no,display_name,stage)
		VALUES ($1,$2,'ROLL-2','Fictional unrecorded learner','PRIMARY')`, unmarked, school)
	exec("", `INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id,class_arm_id,status,created_at,ended_at)
		SELECT $1,tenant_id,$2,session_id,term_id,class_id,class_arm_id,status,created_at,ended_at
		FROM enrollments WHERE tenant_id=$3 AND id=$4`, uuid.New(), unmarked, school, newPlacement)
	summary := func(slug string, offering uuid.UUID, day, credential string, want int) string {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/tenants/"+slug+"/teaching/offerings/"+offering.String()+"/attendance/summary?date="+day, nil)
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("participation summary HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	summary(slug, newOffering, date, "", 401)
	summary(slug, newOffering, date, parentToken, 404)
	summary("lesson-fixture-b", newOffering, date, token, 404)
	summary(slug, newOffering, "2099-01-01", token, 404)
	if body := summary(slug, newOffering, date, token, 200); !strings.Contains(body, `"eligible":2`) ||
		!strings.Contains(body, `"unrecorded":1`) || !strings.Contains(body, `"remote":{"present":1,"absent":0,"late":0,"excused":0}`) ||
		!strings.Contains(body, `"on_site":{"present":0,"absent":0,"late":0,"excused":0}`) || strings.Contains(body, "Fictional") {
		t.Fatal("participation must count corrected marks by mode and keep the unrecorded learner separate")
	}
	if body := summary(slug, oldOffering, date, token, 200); !strings.Contains(body, `"eligible":0`) {
		t.Fatal("the prior arm must not include any of the learners on the new roster")
	}
	exec("", `UPDATE teacher_assignments SET status='INACTIVE' WHERE tenant_id=$1 AND offering_id=$2`, school, newOffering)
	summary(slug, newOffering, date, token, 404)
	managerToken := uuid.NewString()
	exec("", `INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
		SELECT id,$1,$2,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour' FROM users WHERE email='mode-principal@example.test'`,
		auth.HashToken(managerToken), auth.HashToken(csrf))
	notices := func(method, tenant, credential, header, suffix string, payload map[string]any, want int) string {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/v1/tenants/"+tenant+"/notices"+suffix, strings.NewReader(string(body)))
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("school notice HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.String()
	}
	all := map[string]any{"title": "Everyone fixture", "body": "Fictional update", "audience": "SCHOOL"}
	notices("GET", slug, "", "", "", nil, 401)
	notices("POST", slug, managerToken, "", "", all, 403)
	notices("POST", slug, parentToken, csrf, "", all, 404)
	notices("POST", "lesson-fixture-b", managerToken, csrf, "", all, 404)
	notices("POST", slug, managerToken, csrf, "", map[string]any{"title": "Wrong class", "body": "Fixture", "audience": "CLASS", "class_id": uuid.New()}, 422)
	notices("POST", slug, managerToken, csrf, "", all, 201)
	notices("POST", slug, managerToken, csrf, "", map[string]any{"title": "Teacher fixture", "body": "Fixture", "audience": "ROLE", "role": "TEACHER"}, 201)
	notices("POST", slug, managerToken, csrf, "", map[string]any{"title": "Stage fixture", "body": "Fixture", "audience": "STAGE", "stage": "PRIMARY"}, 201)
	notices("POST", slug, managerToken, csrf, "", map[string]any{"title": "Class fixture", "body": "Fixture", "audience": "CLASS", "class_id": class}, 201)
	if body := notices("GET", slug, managerToken, "", "/classes", nil, 200); !strings.Contains(body, class.String()) {
		t.Fatal("manager class choices missing")
	}
	if body := notices("GET", slug, parentToken, "", "/classes", nil, 200); strings.Contains(body, class.String()) {
		t.Fatal("parent saw publishing options")
	}
	if body := notices("GET", slug, token, "", "", nil, 200); !strings.Contains(body, "Teacher fixture") || strings.Contains(body, "Class fixture") {
		t.Fatal("teacher feed ignored its role audience")
	}
	if body := notices("GET", "lesson-fixture-b", parentToken, "", "", nil, 200); strings.Contains(body, "Everyone fixture") {
		t.Fatal("foreign school notice leak")
	}
	if body := notices("GET", slug, parentToken, "", "", nil, 200); !strings.Contains(body, "Everyone fixture") ||
		strings.Contains(body, "Class fixture") || strings.Contains(body, "Stage fixture") || strings.Contains(body, "Teacher fixture") {
		t.Fatal("unlinked guardian received a targeted notice")
	}
	exec("", `INSERT INTO student_guardians (tenant_id,student_id,guardian_id,relationship)
		SELECT $1,e.student_id,$2,'Parent' FROM enrollments e WHERE e.tenant_id=$1 AND e.term_id=$3 AND e.status='ACTIVE' LIMIT 1`,
		school, guardian, term)
	if body := notices("GET", slug, parentToken, "", "", nil, 200); !strings.Contains(body, "Class fixture") || !strings.Contains(body, "Stage fixture") {
		t.Fatal("linked guardian did not receive both enrollment-scoped notices")
	}
	exec("", `UPDATE student_guardians SET active=false WHERE tenant_id=$1 AND guardian_id=$2`, school, guardian)
	if body := notices("GET", slug, parentToken, "", "", nil, 200); strings.Contains(body, "Class fixture") || strings.Contains(body, "Stage fixture") {
		t.Fatal("unlink did not revoke targeted notices")
	}
	if body := get(slug, newOffering, date, token, 200); strings.Contains(body, "Fictional transfer learner") {
		t.Fatal("revoked teacher assignment disclosed a learner")
	}
}

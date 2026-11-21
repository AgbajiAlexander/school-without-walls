package migrations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/modules/people"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testPeopleEnrollmentConstraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("insert fictional fixture: %v", err)
		}
	}
	reject := func(code, query string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, query, args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("expected PostgreSQL error %s, got %v", code, err)
		}
	}

	school, foreignSchool, student, guardian, foreignGuardian := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO tenants (id, slug, name) VALUES ($1, 'people-test-a', 'Fictional A'), ($2, 'people-test-b', 'Fictional B')", school, foreignSchool)
	newStudent := "INSERT INTO students (id, tenant_id, admission_no, display_name, stage) VALUES ($1, $2, $3, 'Fictional Student', 'PRIMARY')"
	exec(newStudent, student, school, "FIXTURE-1")
	reject("23505", newStudent, uuid.New(), school, "fixture-1")
	exec(newStudent, uuid.New(), foreignSchool, "FIXTURE-1")
	exec("INSERT INTO guardians (id, tenant_id, display_name) VALUES ($1, $2, 'Fictional Guardian A'), ($3, $4, 'Fictional Guardian B')", guardian, school, foreignGuardian, foreignSchool)

	newLink := "INSERT INTO student_guardians (tenant_id, student_id, guardian_id, relationship) VALUES ($1, $2, $3, 'guardian')"
	reject("23503", newLink, school, student, foreignGuardian)
	exec(newLink, school, student, guardian)

	parent := uuid.New()
	exec("INSERT INTO users (id, email, password_hash, full_name) VALUES ($1, 'migration-guardian@example.test', 'unused-fixture-hash', 'Fictional Guardian')", parent)
	exec("INSERT INTO memberships (id, tenant_id, user_id, role) VALUES ($1, $2, $3, 'PARENT')", uuid.New(), school, parent)
	exec("UPDATE guardians SET user_id = $1 WHERE id = $2", parent, guardian)
	reject("23503", "UPDATE students SET user_id = $1 WHERE id = $2", parent, student)

	exec("UPDATE tenants SET status = 'ACTIVE' WHERE id IN ($1, $2)", school, foreignSchool)
	exec("UPDATE users SET status = 'ACTIVE' WHERE id = $1", parent)
	owner := uuid.New()
	exec("INSERT INTO users (id, email, password_hash, full_name, status) VALUES ($1, 'migration-owner@example.test', 'unused-fixture-hash', 'Fictional Owner', 'ACTIVE')", owner)
	exec("INSERT INTO memberships (id, tenant_id, user_id, role) VALUES ($1, $2, $3, 'SCHOOL_OWNER')", uuid.New(), school, owner)
	repository := people.NewRepository(pool)
	readLinkedStudent := func() {
		t.Helper()
		summary, err := repository.Student(ctx, "people-test-a", parent, student)
		if err != nil || summary.ID != student {
			t.Fatalf("linked guardian should see only the linked student: %v", err)
		}
	}
	readLinkedStudent()
	if _, err := repository.Student(ctx, "people-test-b", parent, student); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("foreign tenant slug should conceal the student: %v", err)
	}
	unlinkedStudent := uuid.New()
	exec(newStudent, unlinkedStudent, school, "FIXTURE-2")
	if _, err := repository.Student(ctx, "people-test-a", parent, unlinkedStudent); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("guardian should not see an unlinked student in the same tenant: %v", err)
	}
	if err := repository.UnlinkGuardian(ctx, "people-test-a", parent, student, guardian, "parent-denied"); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("guardian should not unlink a relationship: %v", err)
	}
	if err := repository.UnlinkGuardian(ctx, "people-test-a", owner, student, guardian, strings.Repeat("x", 65)); err == nil {
		t.Fatal("invalid audit field should roll back unlink")
	}
	readLinkedStudent()
	router, err := apphttp.NewRouter(apphttp.RouterConfig{
		Config: &config.Config{Environment: "test"},
		DB:     &db.DB{Pool: pool},
	})
	if err != nil {
		t.Fatalf("build people HTTP test router: %v", err)
	}
	ownerToken, guardianToken, csrf := uuid.NewString(), uuid.NewString(), uuid.NewString()
	sessionSQL := "INSERT INTO sessions (user_id, token_hash, csrf_token_hash, ip_address, user_agent, expires_at) VALUES ($1, $2, $3, '127.0.0.1', 'Fictional fixture', CURRENT_TIMESTAMP + interval '1 hour')"
	exec(sessionSQL, owner, auth.HashToken(ownerToken), auth.HashToken(csrf))
	exec(sessionSQL, parent, auth.HashToken(guardianToken), auth.HashToken(csrf))
	studentURL := "/api/v1/tenants/people-test-a/students/" + student.String()
	linkURL := studentURL + "/guardians/" + guardian.String()
	checkHTTP := func(method, url, token, csrfToken string, want int) {
		t.Helper()
		request := httptest.NewRequest(method, url, nil)
		if token != "" {
			request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrfToken != "" {
			request.Header.Set(auth.HeaderCSRF, csrfToken)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("%s people endpoint returned %d, want %d", method, recorder.Code, want)
		}
		if method == http.MethodGet && want == http.StatusOK && recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("student response must not be cacheable")
		}
	}
	checkHTTP(http.MethodGet, studentURL, "", "", http.StatusUnauthorized)
	checkHTTP(http.MethodGet, studentURL, guardianToken, "", http.StatusOK)
	checkHTTP(http.MethodDelete, linkURL, ownerToken, "", http.StatusForbidden)
	checkHTTP(http.MethodDelete, linkURL, guardianToken, csrf, http.StatusNotFound)
	readLinkedStudent()
	checkHTTP(http.MethodDelete, linkURL, ownerToken, csrf, http.StatusNoContent)
	checkHTTP(http.MethodGet, studentURL, guardianToken, "", http.StatusNotFound)
	if _, err := repository.Student(ctx, "people-test-a", parent, student); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("committed unlink should revoke the next guardian read: %v", err)
	}
	if err := repository.UnlinkGuardian(ctx, "people-test-a", owner, student, guardian, "unlink-repeat"); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("repeat unlink should not create another audit event: %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id = $1
		AND action = 'STUDENT_GUARDIAN_UNLINKED' AND subject_id = $2`, school, student.String()).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("expected exactly one committed unlink audit event, got %d: %v", auditCount, err)
	}

	session, otherSession, term, level, class, otherClass, otherArm := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO academic_sessions (id, tenant_id, name, starts_on, ends_on)
	      VALUES ($1, $2, 'Fixture year one', '2030-09-01', '2031-07-31'),
	             ($3, $2, 'Fixture year two', '2031-09-01', '2032-07-31')`, session, school, otherSession)
	exec(`INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on)
	      VALUES ($1, $2, $3, 1, 'Fixture term', '2030-09-01', '2030-12-15')`, term, school, session)
	exec("INSERT INTO grade_levels (id, tenant_id, stage, code, name, sort_order) VALUES ($1, $2, 'PRIMARY', 'P1', 'Fixture Grade', 1)", level, school)
	exec(`INSERT INTO classes (id, tenant_id, session_id, grade_level_id, code, name)
	      VALUES ($1, $2, $3, $4, 'P1', 'Fixture Class'), ($5, $2, $6, $4, 'P1', 'Fixture Other Class')`, class, school, session, level, otherClass, otherSession)
	exec("INSERT INTO class_arms (id, tenant_id, class_id, code, name) VALUES ($1, $2, $3, 'A', 'Fixture Arm')", otherArm, school, otherClass)
	insertEnrollment := "INSERT INTO enrollments (id, tenant_id, student_id, session_id, term_id, class_id, class_arm_id) VALUES ($1, $2, $3, $4, $5, $6, $7)"
	reject("23503", insertEnrollment, uuid.New(), school, student, otherSession, term, otherClass, nil)
	reject("23503", insertEnrollment, uuid.New(), school, student, session, term, class, otherArm)
	first := uuid.New()
	exec(insertEnrollment, first, school, student, session, term, class, nil)
	reject("23505", insertEnrollment, uuid.New(), school, student, session, term, class, nil)
	exec("UPDATE enrollments SET status = 'ENDED', ended_at = CURRENT_TIMESTAMP WHERE id = $1", first)
	current := uuid.New()
	exec(insertEnrollment, current, school, student, session, term, class, nil)
	var active, ended int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status = 'ACTIVE'), count(*) FILTER (WHERE status = 'ENDED')
	    FROM enrollments WHERE tenant_id = $1 AND student_id = $2`, school, student).Scan(&active, &ended); err != nil || active != 1 || ended != 1 {
		t.Fatalf("expected one active and one historical enrollment, got %d/%d: %v", active, ended, err)
	}

	exec(`INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on)
	      VALUES ($1,$3,$4,2,'Fixture second term','2031-01-06','2031-03-31'),
	             ($2,$3,$4,3,'Fixture third term','2031-04-01','2031-07-31')`, uuid.New(), uuid.New(), school, session)
	exec("UPDATE academic_sessions SET status = 'ACTIVE' WHERE id = $1", session)
	arm := uuid.New()
	exec("INSERT INTO class_arms (id, tenant_id, class_id, code, name) VALUES ($1, $2, $3, 'A', 'Transfer Arm')", arm, school, class)
	dest := people.TransferDestination{ClassID: class, ClassArmID: &arm}
	if _, err := repository.TransferEnrollment(ctx, "people-test-a", parent, student, current, dest, "parent-transfer"); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("guardian transfer should be denied: %v", err)
	}
	if _, err := repository.TransferEnrollment(ctx, "people-test-b", owner, student, current, dest, "cross-school"); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("cross-school transfer should be denied: %v", err)
	}
	foreignArm := people.TransferDestination{ClassID: class, ClassArmID: &otherArm}
	if _, err := repository.TransferEnrollment(ctx, "people-test-a", owner, student, current, foreignArm, "invalid-arm"); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("unrelated class arm should be denied: %v", err)
	}
	if _, err := repository.TransferEnrollment(ctx, "people-test-a", owner, student, current, dest, strings.Repeat("x", 65)); err == nil {
		t.Fatal("transfer should fail when its audit cannot be inserted")
	}
	var currentStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM enrollments WHERE id=$1", current).Scan(&currentStatus); err != nil || currentStatus != "ACTIVE" {
		t.Fatalf("failed audit must retain active source: %s, %v", currentStatus, err)
	}
	type outcome struct {
		placement people.Enrollment
		err       error
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			enrollment, err := repository.TransferEnrollment(ctx, "people-test-a", owner, student, current, dest, "transfer-test")
			outcomes <- outcome{enrollment, err}
		}()
	}
	close(start)
	var winner uuid.UUID
	var successes, conflicts int
	for range 2 {
		result := <-outcomes
		switch {
		case result.err == nil:
			winner = result.placement.ID
			successes++
		case errors.Is(result.err, people.ErrEnrollmentConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent transfer result: %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one conflict, got %d/%d", successes, conflicts)
	}
	var contiguous bool
	if err := pool.QueryRow(ctx, `SELECT old.status='ENDED' AND new.status='ACTIVE'
		AND old.ended_at=new.created_at AND new.class_arm_id=$3
		FROM enrollments old CROSS JOIN enrollments new WHERE old.id=$1 AND new.id=$2`, current, winner, arm).Scan(&contiguous); err != nil || !contiguous {
		t.Fatalf("transfer should retain history without a timestamp gap: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1
		AND action='STUDENT_ENROLLMENT_TRANSFERRED' AND subject_id=$2`, school, student.String()).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("expected one committed transfer audit, got %d: %v", auditCount, err)
	}

	transferURL := studentURL + "/enrollments/" + winner.String() + "/transfers"
	checkHTTP(http.MethodPost, transferURL, ownerToken, "", http.StatusForbidden)
	checkHTTP(http.MethodPost, transferURL, ownerToken, csrf, http.StatusUnprocessableEntity)
	makeTransfer := func(token string, want int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, transferURL,
			strings.NewReader(`{"class_id":"`+class.String()+`"}`))
		request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		request.Header.Set(auth.HeaderCSRF, csrf)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("transfer HTTP status %d, want %d", recorder.Code, want)
		}
	}
	makeTransfer(guardianToken, http.StatusNotFound)
	makeTransfer(ownerToken, http.StatusCreated)
	makeTransfer(ownerToken, http.StatusConflict)

	initial := people.InitialEnrollment{TermID: term, TransferDestination: people.TransferDestination{ClassID: class}}
	empty, err := repository.EnrollmentHistory(ctx, "people-test-a", owner, unlinkedStudent, nil)
	if err != nil || empty.Enrollments == nil || len(empty.Enrollments) != 0 {
		t.Fatalf("expected an authorized empty history: %+v, %v", empty, err)
	}
	if _, err := repository.CreateEnrollment(ctx, "people-test-a", owner, unlinkedStudent, initial, strings.Repeat("x", 65)); err == nil {
		t.Fatal("initial enrollment with failing audit should roll back")
	}
	if _, err := repository.CreateEnrollment(ctx, "people-test-a", parent, unlinkedStudent, initial, "denied"); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("guardian must not create enrollment: %v", err)
	}
	historyURL := "/api/v1/tenants/people-test-a/students/" + unlinkedStudent.String() + "/enrollments"
	checkHTTP(http.MethodGet, historyURL, "", "", http.StatusUnauthorized)
	checkHTTP(http.MethodGet, historyURL, guardianToken, "", http.StatusNotFound)
	checkHTTP(http.MethodPost, historyURL, ownerToken, "", http.StatusForbidden)
	checkHTTP(http.MethodPost, historyURL, ownerToken, csrf, http.StatusUnprocessableEntity)

	codes := make(chan int, 2)
	createInitial := func() {
		request := httptest.NewRequest(http.MethodPost, historyURL,
			strings.NewReader(`{"term_id":"`+term.String()+`","class_id":"`+class.String()+`"}`))
		request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: ownerToken})
		request.Header.Set(auth.HeaderCSRF, csrf)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		codes <- recorder.Code
	}
	go createInitial()
	go createInitial()
	a, b := <-codes, <-codes
	if !(a == http.StatusCreated && b == http.StatusConflict || b == http.StatusCreated && a == http.StatusConflict) {
		t.Fatalf("expected one create and one conflict, got %d and %d", a, b)
	}
	created, err := repository.EnrollmentHistory(ctx, "people-test-a", owner, unlinkedStudent, nil)
	if err != nil || len(created.Enrollments) != 1 || created.Enrollments[0].Status != "ACTIVE" || created.Enrollments[0].SessionID != session {
		t.Fatalf("expected one committed active initial placement: %+v, %v", created, err)
	}
	if _, err := repository.EnrollmentHistory(ctx, "people-test-b", owner, unlinkedStudent, nil); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("cross-school history should be concealed: %v", err)
	}
	if _, err := repository.EnrollmentHistory(ctx, "people-test-a", owner, unlinkedStudent, &winner); !errors.Is(err, people.ErrNotFound) {
		t.Fatalf("cursor for another student should be concealed: %v", err)
	}
	exec(`INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id,status,created_at,ended_at)
	      SELECT gen_random_uuid(),$1,$2,$3,$4,$5,'ENDED','2000-01-01'::timestamptz,'2000-01-02'::timestamptz
	      FROM generate_series(1,51)`, school, unlinkedStudent, session, term, class)
	page, err := repository.EnrollmentHistory(ctx, "people-test-a", owner, unlinkedStudent, nil)
	if err != nil || len(page.Enrollments) != 50 || page.NextCursor == nil {
		t.Fatalf("expected bounded history page with cursor: %v", err)
	}
	remaining, err := repository.EnrollmentHistory(ctx, "people-test-a", owner, unlinkedStudent, page.NextCursor)
	if err != nil || len(remaining.Enrollments) != 2 || remaining.NextCursor != nil {
		t.Fatalf("expected final history page: %+v, %v", remaining, err)
	}
	for _, entry := range remaining.Enrollments {
		if entry.Status != "ENDED" || entry.EndedAt == nil {
			t.Fatal("history must retain the ended placement and timestamp")
		}
		for _, earlier := range page.Enrollments {
			if entry.ID == earlier.ID {
				t.Fatal("history cursor repeated a placement")
			}
		}
	}
	checkHTTP(http.MethodGet, historyURL+"?before="+page.NextCursor.String(), ownerToken, "", http.StatusOK)

	studentsURL := "/api/v1/tenants/people-test-a/students"
	checkHTTP(http.MethodGet, studentsURL, "", "", http.StatusUnauthorized)
	checkHTTP(http.MethodGet, studentsURL, guardianToken, "", http.StatusNotFound)
	checkHTTP(http.MethodPost, studentsURL, ownerToken, "", http.StatusForbidden)
	checkHTTP(http.MethodGet, "/api/v1/tenants/people-test-b/students", ownerToken, "", http.StatusNotFound)

	writeProfile := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set(auth.HeaderCSRF, csrf)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("student profile response status %d, want %d", response.Code, want)
		}
		return response
	}
	const profile = `{"admission_no":"  FIXTURE-NEW  ","display_name":" Fictional New ","stage":"PRIMARY"}`
	writeProfile(http.MethodPost, studentsURL, guardianToken, profile, http.StatusNotFound)
	writeProfile(http.MethodPost, studentsURL, ownerToken, `{"admission_no":"NEW","display_name":"Fictional","stage":"UNKNOWN"}`, http.StatusUnprocessableEntity)
	writeProfile(http.MethodPost, studentsURL, ownerToken, `{"admission_no":"NEW","display_name":"Fictional","stage":"PRIMARY","user_id":null}`, http.StatusUnprocessableEntity)
	newResponse := writeProfile(http.MethodPost, studentsURL, ownerToken, profile, http.StatusCreated)
	var createdStudent struct{ Data people.StudentRecord }
	if err := json.Unmarshal(newResponse.Body.Bytes(), &createdStudent); err != nil ||
		createdStudent.Data.ID == uuid.Nil || createdStudent.Data.AdmissionNo != "FIXTURE-NEW" ||
		createdStudent.Data.DisplayName != "Fictional New" {
		t.Fatalf("create must return the normalized committed record: %v", err)
	}
	writeProfile(http.MethodPost, studentsURL, ownerToken, strings.Replace(profile, "FIXTURE-NEW", "fixture-new", 1), http.StatusConflict)
	profileURL := studentsURL + "/" + createdStudent.Data.ID.String()
	writeProfile(http.MethodPut, strings.Replace(profileURL, "people-test-a", "people-test-b", 1), ownerToken, profile, http.StatusNotFound)
	writeProfile(http.MethodPut, profileURL, guardianToken, profile, http.StatusNotFound)
	updated := strings.Replace(profile, "Fictional New", "Fictional Updated", 1)
	writeProfile(http.MethodPut, profileURL, ownerToken, updated, http.StatusOK)
	if _, err := repository.UpdateStudent(ctx, "people-test-a", owner, createdStudent.Data.ID,
		people.StudentInput{AdmissionNo: "FIXTURE-NEW", DisplayName: "Rolled back", Stage: "PRIMARY"}, strings.Repeat("x", 65)); err == nil {
		t.Fatal("student update must roll back if audit fails")
	}
	pageOfStudents, err := repository.ListStudents(ctx, "people-test-a", owner, nil)
	if err != nil || len(pageOfStudents.Students) != 3 {
		t.Fatalf("expected three tenant-scoped student records: %v", err)
	}
	found := false
	for _, row := range pageOfStudents.Students {
		if row.ID == createdStudent.Data.ID {
			found = row.DisplayName == "Fictional Updated"
		}
	}
	if !found {
		t.Fatal("list must show the committed update after audit rollback")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_id=$2
		AND action IN ('STUDENT_PROFILE_CREATED','STUDENT_PROFILE_UPDATED') AND metadata_redacted='{}'::jsonb`,
		school, createdStudent.Data.ID.String()).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("expected exactly two redacted student profile audits: %d, %v", auditCount, err)
	}
	checkHTTP(http.MethodGet, studentsURL, ownerToken, "", http.StatusOK)

	guardiansURL := "/api/v1/tenants/people-test-a/guardians"
	checkHTTP(http.MethodGet, guardiansURL, "", "", http.StatusUnauthorized)
	checkHTTP(http.MethodGet, guardiansURL, guardianToken, "", http.StatusNotFound)
	checkHTTP(http.MethodGet, strings.Replace(guardiansURL, "people-test-a", "people-test-b", 1), ownerToken, "", http.StatusNotFound)
	checkHTTP(http.MethodPost, guardiansURL, ownerToken, "", http.StatusForbidden)
	const guardianProfile = `{"display_name":" Fictional Contact ","contact_email":"fictional-contact@example.test"}`
	writeProfile(http.MethodPost, guardiansURL, ownerToken, `{"display_name":"Fixture","contact_email":"invalid"}`, http.StatusUnprocessableEntity)
	writeProfile(http.MethodPost, guardiansURL, ownerToken, `{"display_name":"Fixture","user_id":null}`, http.StatusUnprocessableEntity)
	profileResponse := writeProfile(http.MethodPost, guardiansURL, ownerToken, guardianProfile, http.StatusCreated)
	var createdGuardian struct{ Data people.GuardianRecord }
	if err := json.Unmarshal(profileResponse.Body.Bytes(), &createdGuardian); err != nil ||
		createdGuardian.Data.DisplayName != "Fictional Contact" || createdGuardian.Data.ContactEmail == nil {
		t.Fatalf("expected a normalized guardian profile: %v", err)
	}
	guardianURL := guardiansURL + "/" + createdGuardian.Data.ID.String()
	writeProfile(http.MethodPut, guardianURL, ownerToken, `{"display_name":"Updated Contact","contact_email":null}`, http.StatusOK)
	checkHTTP(http.MethodGet, guardiansURL, ownerToken, "", http.StatusOK)

	writeProfile(http.MethodPut, studentURL+"/guardians/"+foreignGuardian.String(), ownerToken, `{"relationship":"guardian"}`, http.StatusNotFound)
	writeProfile(http.MethodPut, linkURL, guardianToken, `{"relationship":"guardian"}`, http.StatusNotFound)
	checkHTTP(http.MethodPut, linkURL, ownerToken, "", http.StatusForbidden)
	if err := repository.LinkGuardian(ctx, "people-test-a", owner, student, guardian, "guardian", strings.Repeat("x", 65)); err == nil {
		t.Fatal("failed audit must roll back guardian link reactivation")
	}
	checkHTTP(http.MethodGet, studentURL, guardianToken, "", http.StatusNotFound)
	writeProfile(http.MethodPut, linkURL, ownerToken, `{"relationship":" guardian "}`, http.StatusNoContent)
	writeProfile(http.MethodPut, linkURL, ownerToken, `{"relationship":"guardian"}`, http.StatusNoContent)
	readLinkedStudent()
	linked, err := repository.ListGuardians(ctx, "people-test-a", owner, &student, nil)
	if err != nil || len(linked.Guardians) != 1 || linked.Guardians[0].ID != guardian ||
		linked.Guardians[0].Relationship == nil || *linked.Guardians[0].Relationship != "guardian" {
		t.Fatalf("expected one active guardian relationship: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_id=$2
		AND action='STUDENT_GUARDIAN_LINK_SAVED'`, school, student.String()).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("identical link requests should produce one committed audit: %d, %v", auditCount, err)
	}
	checkHTTP(http.MethodDelete, linkURL, ownerToken, csrf, http.StatusNoContent)
	checkHTTP(http.MethodGet, studentURL, guardianToken, "", http.StatusNotFound)

	staffURL := "/api/v1/tenants/people-test-a/staff"
	checkHTTP(http.MethodGet, staffURL, "", "", http.StatusUnauthorized)
	checkHTTP(http.MethodGet, staffURL, guardianToken, "", http.StatusNotFound)
	checkHTTP(http.MethodPost, staffURL, ownerToken, "", http.StatusForbidden)

	teacher, teacherMembership, foreignMembership := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users (id,email,password_hash,full_name,status) VALUES ($1,'migration-teacher@example.test','unused-fixture-hash','Fictional Teacher','ACTIVE')", teacher)
	exec("INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'TEACHER'),($4,$5,$3,'TEACHER')", teacherMembership, school, teacher, foreignMembership, foreignSchool)
	var parentMembership, ownerMembership uuid.UUID
	for _, person := range []struct {
		user uuid.UUID
		id   *uuid.UUID
	}{{parent, &parentMembership}, {owner, &ownerMembership}} {
		if err := pool.QueryRow(ctx, "SELECT id FROM memberships WHERE tenant_id=$1 AND user_id=$2", school, person.user).Scan(person.id); err != nil {
			t.Fatalf("load fictional membership: %v", err)
		}
	}
	staffBody := func(membership uuid.UUID, code string) string {
		return `{"membership_id":"` + membership.String() + `","staff_code":"` + code + `","display_name":"Fictional Staff"}`
	}
	writeProfile(http.MethodPost, staffURL, ownerToken, staffBody(foreignMembership, "FOREIGN"), http.StatusNotFound)
	writeProfile(http.MethodPost, staffURL, ownerToken, staffBody(parentMembership, "PARENT"), http.StatusNotFound)
	writeProfile(http.MethodPost, staffURL, guardianToken, staffBody(teacherMembership, "TEACHER"), http.StatusNotFound)
	writeProfile(http.MethodPost, staffURL, ownerToken, `{"role":"SCHOOL_ADMIN"}`, http.StatusUnprocessableEntity)
	candidatesURL := staffURL + "/candidates"
	checkHTTP(http.MethodGet, candidatesURL, guardianToken, "", http.StatusNotFound)
	checkHTTP(http.MethodGet, strings.Replace(candidatesURL, "people-test-a", "people-test-b", 1), ownerToken, "", http.StatusNotFound)
	checkHTTP(http.MethodGet, candidatesURL, ownerToken, "", http.StatusOK)
	candidates, err := repository.ListStaffCandidates(ctx, "people-test-a", owner, nil)
	if err != nil || len(candidates.Candidates) != 2 {
		t.Fatalf("expected only owner and teacher as staff candidates: %v", err)
	}
	for _, candidate := range candidates.Candidates {
		if candidate.UserID == teacher && candidate.MembershipID != teacherMembership {
			t.Fatal("staff candidate must use the same-school teacher membership")
		}
	}
	newStaffResponse := writeProfile(http.MethodPost, staffURL, ownerToken, staffBody(teacherMembership, "STAFF-1"), http.StatusCreated)
	var createdStaff struct{ Data people.StaffRecord }
	if err := json.Unmarshal(newStaffResponse.Body.Bytes(), &createdStaff); err != nil || createdStaff.Data.ID == uuid.Nil || createdStaff.Data.UserID != teacher {
		t.Fatalf("staff profile must keep the resolved tenant identity: %v", err)
	}
	writeProfile(http.MethodPost, staffURL, ownerToken, staffBody(teacherMembership, "OTHER-CODE"), http.StatusConflict)
	writeProfile(http.MethodPost, staffURL, ownerToken, staffBody(ownerMembership, "staff-1"), http.StatusConflict)

	recordURL := staffURL + "/" + createdStaff.Data.ID.String()
	writeProfile(http.MethodPut, recordURL, ownerToken, `{"display_name":"Updated Staff","staff_code":"STAFF-2"}`, http.StatusOK)
	if _, err := repository.UpdateStaff(ctx, "people-test-a", owner, createdStaff.Data.ID, people.StaffInput{DisplayName: "Rolled back", StaffCode: "STAFF-3"}, strings.Repeat("x", 65)); err == nil {
		t.Fatal("staff update must roll back if audit fails")
	}
	staff, err := repository.ListStaff(ctx, "people-test-a", owner, nil)
	if err != nil || len(staff.Staff) != 1 || staff.Staff[0].StaffCode != "STAFF-2" {
		t.Fatalf("expected the committed tenant staff update: %v", err)
	}
	exec("UPDATE memberships SET status='SUSPENDED' WHERE id=$1", teacherMembership)
	var activeMemberships int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE tenant_id=$1 AND user_id=$2 AND status='ACTIVE'", school, teacher).Scan(&activeMemberships); err != nil || activeMemberships != 0 {
		t.Fatalf("staff records must not grant or reactivate a membership: %d, %v", activeMemberships, err)
	}
	checkHTTP(http.MethodGet, staffURL, ownerToken, "", http.StatusOK)
	candidates, err = repository.ListStaffCandidates(ctx, "people-test-a", owner, nil)
	if err != nil || len(candidates.Candidates) != 1 || candidates.Candidates[0].UserID != owner {
		t.Fatalf("profiled teacher must disappear from the staff candidate list: %v", err)
	}
}

package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testLessonLifecycle(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	run := func(want, statement string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, statement, args...)
		var dbErr *pgconn.PgError
		if want == "" && err == nil {
			return
		}
		if want == "" || !errors.As(err, &dbErr) || dbErr.Code != want {
			t.Fatalf("lesson SQLSTATE: wanted %q, got %v", want, err)
		}
	}
	school, foreign, user, membership, session, term, grade, class, subject, offering := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	run("", `INSERT INTO tenants (id,slug,name,status) VALUES
		($1,'lesson-fixture-a','Fictional school','ACTIVE'),($2,'lesson-fixture-b','Foreign school','ACTIVE')`, school, foreign)
	run("", `INSERT INTO users (id,email,password_hash,full_name,status)
		VALUES ($1,'lesson-teacher@example.test','unused','Fictional teacher','ACTIVE')`, user)
	run("", `INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'TEACHER')`, membership, school, user)
	run("", `INSERT INTO academic_sessions (id,tenant_id,name,starts_on,ends_on)
		VALUES ($1,$2,'Fictional lesson session',CURRENT_DATE-20,CURRENT_DATE+160)`, session, school)
	run("", `INSERT INTO academic_terms (id,tenant_id,session_id,sequence,name,starts_on,ends_on) VALUES
		($1,$2,$3,1,'First',CURRENT_DATE-20,CURRENT_DATE+20),
		($4,$2,$3,2,'Second',CURRENT_DATE+21,CURRENT_DATE+80),
		($5,$2,$3,3,'Third',CURRENT_DATE+81,CURRENT_DATE+160)`, term, school, session, uuid.New(), uuid.New())
	run("", `INSERT INTO grade_levels (id,tenant_id,stage,code,name,sort_order)
		VALUES ($1,$2,'PRIMARY','LESSON','Fictional Primary',1)`, grade, school)
	run("", `INSERT INTO classes (id,tenant_id,session_id,grade_level_id,code,name)
		VALUES ($1,$2,$3,$4,'LESSON','Fictional Class')`, class, school, session, grade)
	run("", `INSERT INTO subjects (id,tenant_id,code,name) VALUES ($1,$2,'LESSON','Fictional Subject')`, subject, school)
	run("", `INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id)
		VALUES ($1,$2,$3,$4,$5)`, offering, school, subject, term, class)
	first, second, firstObjective, secondObjective, selection, replacement, lesson, pending := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	run("", `INSERT INTO curriculum_framework_versions
		(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note) VALUES
		($1,'NG','Lesson Fixture','Fictional framework','one','NIGERIAN_PRIMARY','2030-01-01','https://example.test/one','Fixture only'),
		($2,'NG','Lesson Fixture','Fictional framework','two','NIGERIAN_PRIMARY','2030-01-01','https://example.test/two','Fixture only')`, first, second)
	run("", `INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text) VALUES
		($1,$2,'PRIMARY','Primary 1','Mathematics','Number','FIX.1','Fictional objective in edition one'),
		($3,$4,'PRIMARY','Primary 1','Mathematics','Number','FIX.1','Fictional objective in edition two')`, firstObjective, first, secondObjective, second)
	run("", `UPDATE curriculum_framework_versions SET status='PUBLISHED' WHERE id IN ($1,$2)`, first, second)
	const choose = `INSERT INTO school_curriculum_selections
		(id,tenant_id,session_id,stage,curriculum_role,revision,framework_version_id,selected_by)
		VALUES ($1,$2,$3,'PRIMARY','PRIMARY',$4,$5,$6)`
	run("", choose, selection, school, session, 1, first, user)
	const create = `INSERT INTO lessons
		(id,tenant_id,offering_id,author_membership_id,curriculum_selection_id,primary_objective_id,title,content)
		VALUES ($1,$2,$3,$4,$5,$6,'Fictional Lesson','Count fictional objects.')`
	run("23514", create, lesson, foreign, offering, membership, selection, firstObjective)
	run("23514", create, lesson, school, offering, membership, selection, secondObjective)
	run("", create, lesson, school, offering, membership, selection, firstObjective)
	run("", create, pending, school, offering, membership, selection, firstObjective)
	const publish = `UPDATE lessons SET status='PUBLISHED' WHERE id=$1`
	run("23514", publish, lesson)
	run("", `UPDATE academic_sessions SET status='ACTIVE' WHERE id=$1`, session)
	run("", choose, replacement, school, session, 2, second, user)
	run("23514", create, uuid.New(), school, offering, membership, selection, firstObjective)
	run("", publish, lesson)
	var pinned uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT curriculum_selection_id FROM lessons WHERE id=$1 AND published_at IS NOT NULL`, lesson).
		Scan(&pinned); err != nil || pinned != selection {
		t.Fatal("publication must retain the original edition selection")
	}
	testStudentLessonHTTP(t, ctx, pool, school, session, term, class, lesson, pending)
	testTeacherLessonHTTP(t, ctx, pool, school, user, membership, offering, secondObjective, firstObjective)
	run("23514", `UPDATE lessons SET content='changed' WHERE id=$1`, lesson)
	run("", `UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, first)
	run("23514", publish, pending)
	run("", `UPDATE lessons SET status='ARCHIVED' WHERE id=$1`, lesson)
	run("23514", `DELETE FROM lessons WHERE id=$1`, lesson)
}

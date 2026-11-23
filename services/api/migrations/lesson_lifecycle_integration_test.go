package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	const createAssignment = `INSERT INTO assignments (id,tenant_id,lesson_id,title,instructions,due_at,max_score,submission_type)
		VALUES ($1,$2,$3,'Counting practice','Count five objects.',clock_timestamp()+interval '1 day',$4,$5)`
	openAssignment := `UPDATE assignments SET status='OPEN' WHERE id=$1`
	assignment, unpublishedAssignment := uuid.New(), uuid.New()
	run("23503", createAssignment, uuid.New(), foreign, lesson, 10, "TEXT")
	run("23514", createAssignment, uuid.New(), school, lesson, 0, "TEXT")
	run("23514", createAssignment, uuid.New(), school, lesson, 10, "UNKNOWN")
	run("", createAssignment, assignment, school, lesson, 10, "TEXT")
	run("", createAssignment, unpublishedAssignment, school, pending, 10, "TEXT")
	run("23514", openAssignment, unpublishedAssignment)
	run("23514", `UPDATE assignments SET status='CLOSED' WHERE id=$1`, assignment)
	run("", `UPDATE assignments SET due_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, assignment)
	run("23514", openAssignment, assignment)
	run("", `UPDATE assignments SET due_at=clock_timestamp()+interval '1 day' WHERE id=$1`, assignment)
	run("", openAssignment, assignment)
	run("23514", `UPDATE assignments SET status='CLOSED',max_score=20 WHERE id=$1`, assignment)
	run("", `UPDATE assignments SET status='CLOSED' WHERE id=$1`, assignment)
	run("23514", openAssignment, assignment)

	const schemeInsert = `INSERT INTO assessment_schemes (id,tenant_id,term_id,stage,version) VALUES ($1,$2,$3,$4,$5)`
	const componentInsert = `INSERT INTO assessment_components (id,tenant_id,scheme_id,name,max_score,weight) VALUES ($1,$2,$3,$4,$5,$6)`
	const publishScheme = `UPDATE assessment_schemes SET status='PUBLISHED' WHERE id=$1`
	scheme, component := uuid.New(), uuid.New()
	run("23503", schemeInsert, uuid.New(), foreign, term, "PRIMARY", 1)
	run("23514", schemeInsert, uuid.New(), school, term, "EARLY_YEARS", 1)
	run("", schemeInsert, scheme, school, term, "PRIMARY", 1)
	run("23514", publishScheme, scheme)
	run("23503", componentInsert, uuid.New(), foreign, scheme, "Foreign fixture", 10, 30)
	run("23514", componentInsert, uuid.New(), school, scheme, "Invalid maximum", 0, 30)
	run("", componentInsert, component, school, scheme, "Fictional coursework", 20, "30.25")
	run("23505", componentInsert, uuid.New(), school, scheme, "fictional coursework", 20, 20)
	run("23514", publishScheme, scheme)
	run("", componentInsert, uuid.New(), school, scheme, "Fictional examination", 60, "69.76")
	run("23514", publishScheme, scheme)
	run("", `UPDATE assessment_components SET weight=69.75 WHERE tenant_id=$1 AND scheme_id=$2 AND id<>$3`, school, scheme, component)
	run("", publishScheme, scheme)
	run("23514", `UPDATE assessment_components SET weight=31 WHERE id=$1`, component)
	run("23514", `DELETE FROM assessment_components WHERE id=$1`, component)
	run("23514", `UPDATE assessment_schemes SET status='DRAFT' WHERE id=$1`, scheme)
	nextScheme, nextComponent := uuid.New(), uuid.New()
	run("", schemeInsert, nextScheme, school, term, "PRIMARY", 2)
	run("", componentInsert, nextComponent, school, nextScheme, "Fictional revised scheme", 10, 100)
	staleTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer staleTx.Rollback(ctx)
	var staleVersion int
	if err := staleTx.QueryRow(ctx, `SELECT version FROM assessment_schemes WHERE id=$1`, nextScheme).Scan(&staleVersion); err != nil {
		t.Fatal(err)
	}
	run("", `UPDATE assessment_components SET weight=90 WHERE id=$1`, nextComponent)
	_, staleErr := staleTx.Exec(ctx, publishScheme, nextScheme)
	var staleDatabaseError *pgconn.PgError
	if !errors.As(staleErr, &staleDatabaseError) || staleDatabaseError.Code != "40001" {
		t.Fatalf("publication with stale assessment weights must fail serialization: %v", staleErr)
	}
	if err := staleTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	run("23514", publishScheme, nextScheme)
	var published bool
	if err := pool.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM assessment_schemes WHERE tenant_id=$1 AND id=$2`, school, scheme).Scan(&published); err != nil || !published {
		t.Fatal("publication must record its database timestamp")
	}
	testSchoolOperatingMode(t, ctx, pool, school, user)
	testStudentLessonHTTP(t, ctx, pool, school, session, term, class, lesson, pending)
	english, extra, alignment := uuid.New(), uuid.New(), uuid.New()
	run("", `INSERT INTO curriculum_framework_versions
		(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note)
		VALUES ($1,'GB-ENG','Lesson Fixture','Fictional enrichment','one','ENRICHMENT','2030-01-01','https://example.test/extra','Fixture only')`, english)
	run("", `INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
		VALUES ($1,$2,'PRIMARY','Year 1','Mathematics','Number','EXTRA.1','Fictional enrichment objective')`, extra, english)
	run("", `UPDATE curriculum_framework_versions SET status='PUBLISHED' WHERE id=$1`, english)
	run("", `INSERT INTO school_curriculum_selections
		(id,tenant_id,session_id,stage,curriculum_role,revision,framework_version_id,selected_by)
		VALUES ($1,$2,$3,'PRIMARY','ENRICHMENT',1,$4,$5)`, uuid.New(), school, session, english, user)
	run("", `INSERT INTO curriculum_objective_alignments (id,primary_objective_id,related_objective_id,relation)
		VALUES ($1,$2,$3,'COMPLEMENTARY')`, alignment, secondObjective, extra)
	enriched := uuid.New()
	run("", create, enriched, school, offering, membership, replacement, secondObjective)
	const attach = `UPDATE lessons SET enrichment_alignment_ids=$2 WHERE id=$1`
	run("23514", attach, enriched, []uuid.UUID{alignment})
	run("", `UPDATE curriculum_objective_alignments SET review_status='HUMAN_REVIEWED',reviewer_id=$2 WHERE id=$1`, alignment, user)
	run("23514", attach, enriched, []uuid.UUID{alignment, alignment})
	run("23514", attach, pending, []uuid.UUID{alignment})
	run("", attach, enriched, []uuid.UUID{alignment})
	var revision int
	if err := pool.QueryRow(ctx, `SELECT content_revision FROM lessons WHERE id=$1`, enriched).Scan(&revision); err != nil || revision != 2 {
		t.Fatal("enrichment must advance the lesson content revision")
	}
	run("", publish, enriched)
	run("23514", attach, enriched, []uuid.UUID{})
	stale := uuid.New()
	run("", create, stale, school, offering, membership, replacement, secondObjective)
	run("", attach, stale, []uuid.UUID{alignment})
	run("", `UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, english)
	run("23514", publish, stale)
	mediaLesson, clean, otherObject := uuid.New(), uuid.New(), uuid.New()
	run("", create, mediaLesson, school, offering, membership, replacement, secondObjective)
	const media = `INSERT INTO lesson_media (id,tenant_id,lesson_id,media_type,size_bytes)
		VALUES ($1,$2,$3,$4,$5)`
	run("23503", media, clean, foreign, mediaLesson, "application/pdf", 30)
	run("23514", media, clean, school, mediaLesson, "text/html", 30)
	run("23514", media, clean, school, mediaLesson, "application/pdf", 8388609)
	run("", media, clean, school, mediaLesson, "application/pdf", 30)
	const setAttachment = `UPDATE lessons SET media_id=$2 WHERE id=$1`
	run("23514", setAttachment, mediaLesson, clean)
	run("", media, otherObject, school, stale, "image/png", 40)
	run("", `UPDATE lesson_media SET status='READY' WHERE id IN ($1,$2)`, clean, otherObject)
	run("23514", setAttachment, mediaLesson, otherObject)
	run("", setAttachment, mediaLesson, clean)
	var mediaRevision int
	if err := pool.QueryRow(ctx, `SELECT content_revision FROM lessons WHERE id=$1`, mediaLesson).Scan(&mediaRevision); err != nil || mediaRevision != 2 {
		t.Fatal("attaching scanned media must invalidate prior lesson review")
	}
	run("", publish, mediaLesson)
	run("23514", setAttachment, mediaLesson, nil)
	run("", `UPDATE lesson_media SET status='REJECTED' WHERE id=$1`, clean)
	run("23514", `UPDATE lesson_media SET status='READY' WHERE id=$1`, clean)
	testTeacherLessonHTTP(t, ctx, pool, school, user, membership, offering, secondObjective, firstObjective)
	testLessonMediaHTTP(t, ctx, pool, school, user, offering, secondObjective, session, term, class)
	testAttendanceRoster(t, ctx, pool, school, foreign, user, membership, session, term, class, subject)
	run("23514", `UPDATE lessons SET content='changed' WHERE id=$1`, lesson)
	run("", `UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, first)
	run("23514", publish, pending)
	run("", `UPDATE lessons SET status='ARCHIVED' WHERE id=$1`, lesson)
	run("23514", `DELETE FROM lessons WHERE id=$1`, lesson)

	student, placement := uuid.New(), uuid.New()
	run("", `INSERT INTO students (id,tenant_id,admission_no,display_name,stage)
		VALUES ($1,$2,'FIX-SCORE','Fictional Learner','PRIMARY')`, student, school)
	run("", `INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id)
		VALUES ($1,$2,$3,$4,$5,$6)`, placement, school, student, session, term, class)
	const scoreInsert = `INSERT INTO assessment_scores
		(id,tenant_id,enrollment_id,offering_id,component_id,score) VALUES ($1,$2,$3,$4,$5,$6)`
	score := uuid.New()
	run("23514", scoreInsert, score, foreign, placement, offering, component, "0")
	run("23514", scoreInsert, score, school, placement, offering, component, "-0.01")
	run("23514", scoreInsert, score, school, placement, offering, component, "20.01")
	run("23514", scoreInsert, score, school, placement, offering, component, "19.999")
	run("", scoreInsert, score, school, placement, offering, component, "0")
	run("23505", scoreInsert, uuid.New(), school, placement, offering, component, "0")
	run("", `UPDATE assessment_scores SET score=20.00 WHERE id=$1`, score)
	run("23514", `UPDATE assessment_scores SET score=20.01 WHERE id=$1`, score)
	run("", `DELETE FROM assessment_scores WHERE id=$1`, score)
	testAssessmentScoreHTTP(t, ctx, pool, school, user, offering, component, placement)
}

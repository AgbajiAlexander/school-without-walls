package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testCurriculumAlignmentReview(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	run := func(want, query string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, query, args...)
		if want == "" && err == nil {
			return
		}
		var databaseError *pgconn.PgError
		if want == "" || !errors.As(err, &databaseError) || databaseError.Code != want {
			t.Fatalf("alignment SQLSTATE: wanted %q, got %v", want, err)
		}
	}
	primaryEdition, relatedEdition, primary, related, reviewer, mapping := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	run("", `INSERT INTO curriculum_framework_versions
		(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note) VALUES
		($1,'NG','Alignment fixture','Fictional Nigerian','one','NIGERIAN_PRIMARY','2030-01-01','https://example.test/primary','Fixture only'),
		($2,'GB-ENG','Alignment fixture','Fictional Enrichment','one','ENRICHMENT','2030-01-01','https://example.test/enrichment','Fixture only')`,
		primaryEdition, relatedEdition)
	run("", `INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text) VALUES
		($1,$2,'PRIMARY','Primary 1','Mathematics','Number','FIX.1','Fictional Nigerian objective'),
		($3,$4,'PRIMARY','Year 1','Mathematics','Number','FIX.1','Fictional enrichment objective')`,
		primary, primaryEdition, related, relatedEdition)
	run("", `INSERT INTO users (id,email,password_hash,full_name,status)
		VALUES ($1,'alignment-reviewer@example.test','unused','Fictional Reviewer','ACTIVE')`, reviewer)

	const insert = `INSERT INTO curriculum_objective_alignments (id,primary_objective_id,related_objective_id,relation)
		VALUES ($1,$2,$3,$4)`
	run("23514", insert, mapping, related, primary, "COMPLEMENTARY")
	run("23514", insert, mapping, primary, related, "UNRECOGNIZED")
	run("", insert, mapping, primary, related, "COMPLEMENTARY")
	run("23505", insert, uuid.New(), primary, related, "EXTENDS")

	checkVerified := func(want int) {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM curriculum_verified_alignments WHERE id=$1`, mapping).
			Scan(&count); err != nil || count != want {
			t.Fatalf("verified mapping count: got %d, want %d (%v)", count, want, err)
		}
	}
	checkVerified(0)
	const review = `UPDATE curriculum_objective_alignments SET review_status='HUMAN_REVIEWED', reviewer_id=$2 WHERE id=$1`
	run("23514", review, mapping, reviewer)
	run("", `UPDATE curriculum_framework_versions SET status='PUBLISHED' WHERE id IN ($1,$2)`, primaryEdition, relatedEdition)
	run("23514", review, mapping, nil)
	run("23514", `UPDATE curriculum_objective_alignments SET relation='EXTENDS', review_status='HUMAN_REVIEWED', reviewer_id=$2 WHERE id=$1`,
		mapping, reviewer)
	run("", review, mapping, reviewer)
	checkVerified(1)
	run("23514", `UPDATE curriculum_objective_alignments SET relation='DIFFERENT' WHERE id=$1`, mapping)
	run("23514", `DELETE FROM curriculum_objective_alignments WHERE id=$1`, mapping)
	run("", `UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, relatedEdition)
	checkVerified(0)
	var retained bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM curriculum_objective_alignments
		WHERE id=$1 AND reviewer_id=$2 AND reviewed_at IS NOT NULL AND review_status='HUMAN_REVIEWED')`,
		mapping, reviewer).Scan(&retained); err != nil || !retained {
		t.Fatal("withdrawal must keep the original reviewer and review date")
	}
}

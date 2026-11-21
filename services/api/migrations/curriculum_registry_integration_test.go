package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testCurriculumRegistry(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	exec := func(want, sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		if want == "" && err == nil {
			return
		}
		var pgErr *pgconn.PgError
		if want == "" || !errors.As(err, &pgErr) || pgErr.Code != want {
			t.Fatalf("curriculum constraint: wanted SQLSTATE %q, got %v", want, err)
		}
	}
	const framework = `INSERT INTO curriculum_framework_versions
		(id, jurisdiction, publisher, title, edition, framework_kind, effective_from, source_url, license_note)
		VALUES ($1,$2,'Fictional Publisher','Fictional Curriculum',$3,$4,'2030-01-01','https://curriculum.example.test','Fictional test content only')`
	const objective = `INSERT INTO curriculum_objectives
		(id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
		VALUES ($1,$2,'PRIMARY','Primary 1','Mathematics','Number','COUNT.1',$3)`
	original, revised, enrichment := uuid.New(), uuid.New(), uuid.New()
	exec("", framework, original, "NG", "first", "NIGERIAN_PRIMARY")
	exec("", framework, revised, "NG", "second", "NIGERIAN_PRIMARY")
	exec("", framework, enrichment, "GB-ENG", "first", "ENRICHMENT")
	exec("23505", framework, uuid.New(), "NG", "FIRST", "NIGERIAN_PRIMARY")
	exec("23514", framework, uuid.New(), "GB-ENG", "second", "NIGERIAN_PRIMARY")

	firstObjective, secondObjective := uuid.New(), uuid.New()
	exec("", objective, firstObjective, original, "Count one group of fictional objects.")
	exec("", objective, secondObjective, revised, "Count and compare two fictional groups.")
	exec("23505", objective, uuid.New(), original, "Duplicate code.")
	exec("23514", `UPDATE curriculum_objectives SET framework_version_id = $1 WHERE id = $2`, revised, firstObjective)
	exec("", `UPDATE curriculum_framework_versions SET status = 'PUBLISHED' WHERE id = $1`, original)
	exec("23514", `UPDATE curriculum_framework_versions SET edition = 'replacement' WHERE id = $1`, original)
	exec("23514", `UPDATE curriculum_objectives SET objective_text = 'Replacement' WHERE id = $1`, firstObjective)
	exec("23514", objective, uuid.New(), original, "Late addition.")
	exec("23514", `DELETE FROM curriculum_objectives WHERE id = $1`, firstObjective)
	exec("", `UPDATE curriculum_framework_versions SET status = 'WITHDRAWN' WHERE id = $1`, original)
	exec("23514", `UPDATE curriculum_framework_versions SET status = 'PUBLISHED' WHERE id = $1`, original)
	exec("23514", `DELETE FROM curriculum_framework_versions WHERE id = $1`, original)
	var retained string
	if err := pool.QueryRow(ctx, `SELECT objective_text FROM curriculum_objectives WHERE id = $1`, firstObjective).Scan(&retained); err != nil ||
		retained != "Count one group of fictional objects." {
		t.Fatal("original objective must remain available to historical references")
	}
}

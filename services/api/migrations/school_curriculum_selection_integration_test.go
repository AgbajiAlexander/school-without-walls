package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testSchoolCurriculumSelection(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	exec := func(want, sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		if want == "" && err == nil {
			return
		}
		var dbErr *pgconn.PgError
		if want == "" || !errors.As(err, &dbErr) || dbErr.Code != want {
			t.Fatalf("selection constraint: wanted %q, got %v", want, err)
		}
	}
	school, foreign, session, actor := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("", `INSERT INTO tenants (id, slug, name, status) VALUES
		($1,'choice-fixture-a','Fictional school A','ACTIVE'),($2,'choice-fixture-b','Fictional school B','ACTIVE')`, school, foreign)
	exec("", `INSERT INTO academic_sessions (id,tenant_id,name,starts_on,ends_on)
		VALUES ($1,$2,'Fictional session','2030-09-01','2031-07-31')`, session, school)
	exec("", `INSERT INTO users (id,email,password_hash,full_name,status)
		VALUES ($1,'selection-fixture@example.test','unused','Fictional manager','ACTIVE')`, actor)
	const framework = `INSERT INTO curriculum_framework_versions
		(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note)
		VALUES ($1,$2,'Selection Fixture','Fictional Framework',$3,$4,'2030-01-01','https://example.test/curriculum','Fixture only')`
	const objective = `INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
		VALUES ($1,$2,$3,'Fixture band','Mathematics','Number','FIX.1','Fictional objective')`
	first, second, enrichment, developmental := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("", framework, first, "NG", "first", "NIGERIAN_PRIMARY")
	exec("", framework, second, "NG", "second", "NIGERIAN_PRIMARY")
	exec("", framework, enrichment, "GB-ENG", "first", "ENRICHMENT")
	exec("", framework, developmental, "GB-ENG", "early", "DEVELOPMENTAL")
	for _, edition := range []uuid.UUID{first, second, enrichment} {
		exec("", objective, uuid.New(), edition, "PRIMARY")
	}
	exec("", objective, uuid.New(), developmental, "EARLY_YEARS")
	const publish = `UPDATE curriculum_framework_versions SET status='PUBLISHED' WHERE id=$1`
	exec("", publish, first)
	exec("", publish, enrichment)
	exec("", publish, developmental)
	const choose = `INSERT INTO school_curriculum_selections
		(id,tenant_id,session_id,stage,curriculum_role,revision,framework_version_id,selected_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	initial := uuid.New()
	exec("23503", choose, initial, foreign, session, "PRIMARY", "PRIMARY", 1, first, actor)
	exec("23514", choose, initial, school, session, "PRIMARY", "PRIMARY", 1, second, actor)
	exec("23514", choose, initial, school, session, "PRIMARY", "PRIMARY", 1, enrichment, actor)
	exec("23514", choose, initial, school, session, "EARLY_YEARS", "PRIMARY", 1, first, actor)
	exec("23514", choose, initial, school, session, "PRIMARY", "PRIMARY", 2, first, actor)
	exec("", choose, initial, school, session, "PRIMARY", "PRIMARY", 1, first, actor)
	exec("", choose, uuid.New(), school, session, "PRIMARY", "ENRICHMENT", 1, enrichment, actor)
	exec("", choose, uuid.New(), school, session, "EARLY_YEARS", "PRIMARY", 1, developmental, actor)
	exec("23514", choose, uuid.New(), school, session, "PRIMARY", "PRIMARY", 2, first, actor)
	exec("", publish, second)
	exec("", choose, uuid.New(), school, session, "PRIMARY", "PRIMARY", 2, second, actor)
	exec("23514", `UPDATE school_curriculum_selections SET framework_version_id=$2 WHERE id=$1`, initial, second)
	exec("23514", `DELETE FROM school_curriculum_selections WHERE id=$1`, initial)
	exec("", `UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, second)
	var historical, current uuid.UUID
	err := pool.QueryRow(ctx, `SELECT original.framework_version_id, current.framework_version_id
		FROM school_curriculum_selections original
		JOIN school_current_curriculum_selections current USING (tenant_id, session_id, stage, curriculum_role)
		WHERE original.id=$1`, initial).Scan(&historical, &current)
	if err != nil || historical != first || current != second {
		t.Fatal("history should retain the original edition and the current choice should remain explicit after withdrawal")
	}
	exec("23514", choose, uuid.New(), school, session, "PRIMARY", "PRIMARY", 3, second, actor)
	t.Run("http", func(t *testing.T) {
		testSchoolCurriculumHTTP(t, ctx, pool, school, session, actor, first, second)
	})
	exec("23514", choose, uuid.New(), school, session, "PRIMARY", "PRIMARY", 4, first, actor)
}

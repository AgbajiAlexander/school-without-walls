package people

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrAdmissionNumberConflict = errors.New("admission number already exists")

type StudentInput struct {
	AdmissionNo string `json:"admission_no"`
	DisplayName string `json:"display_name"`
	Stage       string `json:"stage"`
}

type StudentRecord struct {
	StudentSummary
	AdmissionNo string `json:"admission_no"`
	Status      string `json:"status"`
}

type StudentPage struct {
	Students   []StudentRecord `json:"students"`
	NextCursor *uuid.UUID      `json:"next_cursor,omitempty"`
}

func (input *StudentInput) normalize() bool {
	input.AdmissionNo = strings.TrimSpace(input.AdmissionNo)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	switch input.Stage {
	case "EARLY_YEARS", "PRIMARY", "JUNIOR_SECONDARY", "SENIOR_SECONDARY":
	default:
		return false
	}
	return validProfileText(input.AdmissionNo, 40) && validProfileText(input.DisplayName, 150)
}

func validProfileText(value string, max int) bool {
	return value != "" && utf8.RuneCountInString(value) <= max && strings.IndexFunc(value, unicode.IsControl) == -1
}

func (r *Repository) ListStudents(ctx context.Context, slug string, actor uuid.UUID, after *uuid.UUID) (StudentPage, error) {
	const limit = 50
	var page StudentPage
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(page) ORDER BY page.id)
		                FILTER (WHERE page.id IS NOT NULL), '[]'::jsonb)
		FROM tenants t
		JOIN users u ON u.id=$2 AND u.status='ACTIVE'
		LEFT JOIN LATERAL (
			SELECT id,display_name,stage,admission_no,status FROM students
			WHERE tenant_id=t.id AND ($3::uuid IS NULL OR id>$3)
			ORDER BY id LIMIT $4
		) page ON true
		WHERE t.slug=$1::citext AND t.status='ACTIVE'
		  AND EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id=t.id AND m.user_id=u.id
		      AND m.status='ACTIVE' AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL'))
		GROUP BY t.id
	`, slug, actor, after, limit+1).Scan(&page.Students)
	if err != nil {
		return StudentPage{}, scopeError(err)
	}
	if len(page.Students) > limit {
		page.Students = page.Students[:limit]
		last := page.Students[limit-1].ID
		page.NextCursor = &last
	}
	return page, nil
}

func (r *Repository) CreateStudent(ctx context.Context, slug string, actor uuid.UUID, input StudentInput, requestID string) (StudentRecord, error) {
	return r.writeStudent(ctx, slug, actor, uuid.New(), input, requestID, true)
}

func (r *Repository) UpdateStudent(ctx context.Context, slug string, actor, student uuid.UUID, input StudentInput, requestID string) (StudentRecord, error) {
	return r.writeStudent(ctx, slug, actor, student, input, requestID, false)
}

func (r *Repository) writeStudent(ctx context.Context, slug string, actor, student uuid.UUID, input StudentInput, requestID string, create bool) (StudentRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return StudentRecord{}, err
	}
	defer tx.Rollback(ctx)
	tenant, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return StudentRecord{}, err
	}

	query := `UPDATE students SET admission_no=$3, display_name=$4, stage=$5, updated_at=clock_timestamp()
	          WHERE tenant_id=$1 AND id=$2`
	action := "STUDENT_PROFILE_UPDATED"
	if create {
		query = `INSERT INTO students(tenant_id,id,admission_no,display_name,stage) VALUES($1,$2,$3,$4,$5)`
		action = "STUDENT_PROFILE_CREATED"
	}
	var record StudentRecord
	err = tx.QueryRow(ctx, query+" RETURNING id,display_name,stage,admission_no,status",
		tenant, student, input.AdmissionNo, input.DisplayName, input.Stage).
		Scan(&record.ID, &record.DisplayName, &record.Stage, &record.AdmissionNo, &record.Status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "students_tenant_id_admission_no_key" {
			return StudentRecord{}, ErrAdmissionNumberConflict
		}
		return StudentRecord{}, scopeError(err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
		VALUES ($1,$2,$3,$4,'student',$5,$6)
	`, uuid.New(), tenant, actor, action, student.String(), requestID)
	if err != nil {
		return StudentRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StudentRecord{}, err
	}
	return record, nil
}

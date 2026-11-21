package people

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type InitialEnrollment struct {
	TermID uuid.UUID `json:"term_id"`
	TransferDestination
}

type EnrollmentHistoryEntry struct {
	Enrollment
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	EndedAt      *time.Time `json:"ended_at"`
	SessionName  string     `json:"session_name"`
	TermName     string     `json:"term_name"`
	ClassName    string     `json:"class_name"`
	ClassArmName *string    `json:"class_arm_name"`
}

type EnrollmentHistory struct {
	Enrollments []EnrollmentHistoryEntry `json:"enrollments"`
	NextCursor  *uuid.UUID               `json:"next_cursor,omitempty"`
}

func (r *Repository) CreateEnrollment(ctx context.Context, slug string, actor, studentID uuid.UUID, input InitialEnrollment, requestID string) (Enrollment, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Enrollment{}, err
	}
	defer tx.Rollback(ctx)
	tenant, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return Enrollment{}, err
	}
	if err := lockActiveStudent(ctx, tx, tenant, studentID); err != nil {
		return Enrollment{}, err
	}

	var sessionID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT a.id
		FROM academic_terms term
		JOIN academic_sessions a ON a.tenant_id=term.tenant_id AND a.id=term.session_id
		WHERE term.tenant_id=$1 AND term.id=$2 AND a.status='ACTIVE'
		FOR SHARE OF a
	`, tenant, input.TermID).Scan(&sessionID); err != nil {
		return Enrollment{}, scopeError(err)
	}
	if err := lockDestination(ctx, tx, tenant, sessionID, input.TransferDestination); err != nil {
		return Enrollment{}, err
	}

	placement := Enrollment{ID: uuid.New(), StudentID: studentID, SessionID: sessionID, TermID: input.TermID, ClassID: input.ClassID, ClassArmID: input.ClassArmID}
	result, err := tx.Exec(ctx, `
		INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id,class_arm_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (tenant_id,student_id,term_id) WHERE status='ACTIVE' DO NOTHING
	`, placement.ID, tenant, studentID, sessionID, input.TermID, input.ClassID, input.ClassArmID)
	if err != nil {
		return Enrollment{}, err
	}
	if result.RowsAffected() == 0 {
		return Enrollment{}, ErrEnrollmentConflict
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
		VALUES ($1,$2,$3,'STUDENT_ENROLLMENT_CREATED','student',$4,$5,
		        jsonb_build_object('enrollment_id',$6::text))
	`, uuid.New(), tenant, actor, studentID.String(), requestID, placement.ID.String())
	if err != nil {
		return Enrollment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Enrollment{}, err
	}
	return placement, nil
}

func (r *Repository) EnrollmentHistory(ctx context.Context, slug string, actor, studentID uuid.UUID, before *uuid.UUID) (EnrollmentHistory, error) {
	const pageSize = 50
	var history EnrollmentHistory
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(page) ORDER BY page.created_at DESC, page.id DESC)
		                FILTER (WHERE page.id IS NOT NULL), '[]'::jsonb)
		FROM students s
		JOIN tenants t ON t.id=s.tenant_id AND t.status='ACTIVE'
		JOIN users u ON u.id=$2 AND u.status='ACTIVE'
		LEFT JOIN enrollments anchor ON anchor.tenant_id=s.tenant_id AND anchor.student_id=s.id AND anchor.id=$4
		LEFT JOIN LATERAL (
			SELECT e.id,e.student_id,e.session_id,e.term_id,e.class_id,e.class_arm_id,e.status,e.created_at,e.ended_at,
			       session.name AS session_name, term.name AS term_name, class.name AS class_name, arm.name AS class_arm_name
			FROM enrollments e
			JOIN academic_sessions session ON session.tenant_id=e.tenant_id AND session.id=e.session_id
			JOIN academic_terms term ON term.tenant_id=e.tenant_id AND term.id=e.term_id
			JOIN classes class ON class.tenant_id=e.tenant_id AND class.id=e.class_id
			LEFT JOIN class_arms arm ON arm.tenant_id=e.tenant_id AND arm.id=e.class_arm_id
			WHERE e.tenant_id=s.tenant_id AND e.student_id=s.id
			  AND ($4::uuid IS NULL OR (e.created_at,e.id) < (anchor.created_at,anchor.id))
			ORDER BY e.created_at DESC,e.id DESC LIMIT $5
		) page ON true
		WHERE t.slug=$1::citext AND s.id=$3 AND ($4::uuid IS NULL OR anchor.id IS NOT NULL)
		  AND EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id=s.tenant_id AND m.user_id=u.id
		      AND m.status='ACTIVE' AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL'))
		GROUP BY s.id
	`, slug, actor, studentID, before, pageSize+1).Scan(&history.Enrollments)
	if err != nil {
		return EnrollmentHistory{}, scopeError(err)
	}
	if len(history.Enrollments) > pageSize {
		history.Enrollments = history.Enrollments[:pageSize]
		last := history.Enrollments[pageSize-1].ID
		history.NextCursor = &last
	}
	return history, nil
}

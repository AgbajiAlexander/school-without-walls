package people

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrEnrollmentConflict = errors.New("enrollment changed or destination is unchanged")

type TransferDestination struct {
	ClassID    uuid.UUID  `json:"class_id"`
	ClassArmID *uuid.UUID `json:"class_arm_id"`
}

type Enrollment struct {
	ID         uuid.UUID  `json:"id"`
	StudentID  uuid.UUID  `json:"student_id"`
	SessionID  uuid.UUID  `json:"session_id"`
	TermID     uuid.UUID  `json:"term_id"`
	ClassID    uuid.UUID  `json:"class_id"`
	ClassArmID *uuid.UUID `json:"class_arm_id"`
}

// TransferEnrollment requires the expected active record, so concurrent or
// retried requests cannot silently transfer an already-moved student.
func (r *Repository) TransferEnrollment(ctx context.Context, slug string, actor, studentID, expectedID uuid.UUID, destination TransferDestination, requestID string) (Enrollment, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Enrollment{}, err
	}
	defer tx.Rollback(ctx)

	tenant, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return Enrollment{}, err
	}
	var current Enrollment
	if err := lockActiveStudent(ctx, tx, tenant, studentID); err != nil {
		return Enrollment{}, err
	}
	var sessionID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT a.id FROM academic_sessions a
		JOIN enrollments e ON e.tenant_id = a.tenant_id AND e.session_id = a.id
		WHERE e.tenant_id = $1 AND e.student_id = $2 AND e.id = $3 AND a.status = 'ACTIVE'
		FOR SHARE OF a
	`, tenant, studentID, expectedID).Scan(&sessionID); err != nil {
		return Enrollment{}, scopeError(err)
	}
	var state string
	err = tx.QueryRow(ctx, `
		SELECT session_id, term_id, class_id, class_arm_id, status
		FROM enrollments WHERE tenant_id=$1 AND student_id=$2 AND id=$3 AND session_id=$4 FOR UPDATE
	`, tenant, studentID, expectedID, sessionID).Scan(&current.SessionID, &current.TermID, &current.ClassID, &current.ClassArmID, &state)
	if err != nil {
		return Enrollment{}, scopeError(err)
	}
	if state != "ACTIVE" || (current.ClassID == destination.ClassID && sameArm(current.ClassArmID, destination.ClassArmID)) {
		return Enrollment{}, ErrEnrollmentConflict
	}
	if err := lockDestination(ctx, tx, tenant, current.SessionID, destination); err != nil {
		return Enrollment{}, err
	}

	replacement := Enrollment{ID: uuid.New(), StudentID: studentID, SessionID: current.SessionID, TermID: current.TermID, ClassID: destination.ClassID, ClassArmID: destination.ClassArmID}
	// Use the same database timestamp for ending and starting the placements,
	// sampled after locks so it cannot predate a recently created placement.
	_, err = tx.Exec(ctx, `
		WITH ended AS (
			UPDATE enrollments SET status='ENDED', ended_at=clock_timestamp()
			WHERE tenant_id=$1 AND id=$2
			RETURNING ended_at
		)
		INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id,class_arm_id,created_at)
		SELECT $3,$1,$4,$5,$6,$7,$8,ended_at FROM ended
	`, tenant, expectedID, replacement.ID, studentID, current.SessionID, current.TermID, destination.ClassID, destination.ClassArmID)
	if err != nil {
		return Enrollment{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
		VALUES ($1,$2,$3,'STUDENT_ENROLLMENT_TRANSFERRED','student',$4,$5,
		        jsonb_build_object('previous_enrollment_id',$6::text,'new_enrollment_id',$7::text))
	`, uuid.New(), tenant, actor, studentID.String(), requestID, expectedID.String(), replacement.ID.String())
	if err != nil {
		return Enrollment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Enrollment{}, err
	}
	return replacement, nil
}

func sameArm(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func lockActiveStudent(ctx context.Context, tx pgx.Tx, tenant, studentID uuid.UUID) error {
	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE' FOR UPDATE`, tenant, studentID).Scan(&lockedID); err != nil {
		return scopeError(err)
	}
	return nil
}

func lockDestination(ctx context.Context, tx pgx.Tx, tenant, sessionID uuid.UUID, destination TransferDestination) error {
	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM classes WHERE tenant_id=$1 AND session_id=$2 AND id=$3 AND status='ACTIVE' FOR SHARE
	`, tenant, sessionID, destination.ClassID).Scan(&lockedID); err != nil {
		return scopeError(err)
	}
	if destination.ClassArmID != nil {
		if err := tx.QueryRow(ctx, `
			SELECT id FROM class_arms WHERE tenant_id=$1 AND class_id=$2 AND id=$3 AND status='ACTIVE' FOR SHARE
		`, tenant, destination.ClassID, *destination.ClassArmID).Scan(&lockedID); err != nil {
			return scopeError(err)
		}
	}

	return nil
}

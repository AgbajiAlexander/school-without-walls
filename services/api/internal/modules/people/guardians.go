package people

import (
	"context"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GuardianInput struct {
	DisplayName  string  `json:"display_name"`
	ContactEmail *string `json:"contact_email"`
}

type GuardianRecord struct {
	ID           uuid.UUID `json:"id"`
	DisplayName  string    `json:"display_name"`
	ContactEmail *string   `json:"contact_email"`
	Relationship *string   `json:"relationship,omitempty"`
}

type GuardianPage struct {
	Guardians  []GuardianRecord `json:"guardians"`
	NextCursor *uuid.UUID       `json:"next_cursor,omitempty"`
}

func (input *GuardianInput) normalize() bool {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !validProfileText(input.DisplayName, 150) {
		return false
	}
	if input.ContactEmail == nil {
		return true
	}
	raw := strings.TrimSpace(*input.ContactEmail)
	if raw == "" {
		input.ContactEmail = nil
		return true
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || address.Address != raw || !validProfileText(raw, 254) || len(raw) > 254 {
		return false
	}
	input.ContactEmail = &raw
	return true
}

func (r *Repository) ListGuardians(ctx context.Context, slug string, actor uuid.UUID, student, after *uuid.UUID) (GuardianPage, error) {
	const limit = 50
	var result GuardianPage
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(page) ORDER BY page.id)
		                FILTER (WHERE page.id IS NOT NULL), '[]'::jsonb)
		FROM tenants t
		JOIN users u ON u.id=$2 AND u.status='ACTIVE'
		LEFT JOIN LATERAL (
			SELECT g.id,g.display_name,g.contact_email,link.relationship
			FROM guardians g
			LEFT JOIN student_guardians link ON link.tenant_id=g.tenant_id
			  AND link.guardian_id=g.id AND link.student_id=$3 AND link.active
			WHERE g.tenant_id=t.id AND ($4::uuid IS NULL OR g.id>$4)
			  AND ($3::uuid IS NULL OR link.student_id IS NOT NULL)
			ORDER BY g.id LIMIT $5
		) page ON true
		WHERE t.slug=$1::citext AND t.status='ACTIVE'
		  AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM students s WHERE s.tenant_id=t.id AND s.id=$3))
		  AND EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id=t.id AND m.user_id=u.id
		      AND m.status='ACTIVE' AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL'))
		GROUP BY t.id
	`, slug, actor, student, after, limit+1).Scan(&result.Guardians)
	if err != nil {
		return GuardianPage{}, scopeError(err)
	}
	if len(result.Guardians) > limit {
		result.Guardians = result.Guardians[:limit]
		last := result.Guardians[limit-1].ID
		result.NextCursor = &last
	}
	return result, nil
}

func (r *Repository) CreateGuardian(ctx context.Context, slug string, actor uuid.UUID, input GuardianInput, requestID string) (GuardianRecord, error) {
	return r.writeGuardian(ctx, slug, actor, uuid.New(), input, requestID, true)
}

func (r *Repository) UpdateGuardian(ctx context.Context, slug string, actor, guardian uuid.UUID, input GuardianInput, requestID string) (GuardianRecord, error) {
	return r.writeGuardian(ctx, slug, actor, guardian, input, requestID, false)
}

func (r *Repository) writeGuardian(ctx context.Context, slug string, actor, guardian uuid.UUID, input GuardianInput, requestID string, create bool) (GuardianRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return GuardianRecord{}, err
	}
	defer tx.Rollback(ctx)
	tenant, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return GuardianRecord{}, err
	}

	query := `UPDATE guardians SET display_name=$3,contact_email=$4,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`
	action := "GUARDIAN_PROFILE_UPDATED"
	if create {
		query = `INSERT INTO guardians (tenant_id,id,display_name,contact_email) VALUES($1,$2,$3,$4)`
		action = "GUARDIAN_PROFILE_CREATED"
	}
	var record GuardianRecord
	err = tx.QueryRow(ctx, query+" RETURNING id,display_name,contact_email", tenant, guardian, input.DisplayName, input.ContactEmail).
		Scan(&record.ID, &record.DisplayName, &record.ContactEmail)
	if err != nil {
		return GuardianRecord{}, scopeError(err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
		VALUES ($1,$2,$3,$4,'guardian',$5,$6)
	`, uuid.New(), tenant, actor, action, guardian.String(), requestID)
	if err != nil {
		return GuardianRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GuardianRecord{}, err
	}
	return record, nil
}

// LinkGuardian registers or reactivates the relationship without touching identity or membership.
func (r *Repository) LinkGuardian(ctx context.Context, slug string, actor, student, guardian uuid.UUID, relationship, requestID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tenant, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return err
	}
	if err := lockActiveStudent(ctx, tx, tenant, student); err != nil {
		return err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM guardians WHERE tenant_id=$1 AND id=$2 FOR SHARE`, tenant, guardian).Scan(&id); err != nil {
		return scopeError(err)
	}

	result, err := tx.Exec(ctx, `
		INSERT INTO student_guardians (tenant_id,student_id,guardian_id,relationship)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (tenant_id,student_id,guardian_id) DO UPDATE
		SET relationship=EXCLUDED.relationship,active=true,updated_at=clock_timestamp()
		WHERE NOT student_guardians.active OR student_guardians.relationship IS DISTINCT FROM EXCLUDED.relationship
	`, tenant, student, guardian, relationship)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
			VALUES ($1,$2,$3,'STUDENT_GUARDIAN_LINK_SAVED','student',$4,$5,jsonb_build_object('guardian_id',$6::text))
		`, uuid.New(), tenant, actor, student.String(), requestID, guardian.String())
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

package people

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrStaffProfileConflict = errors.New("staff code or identity already has a profile")

type StaffInput struct {
	DisplayName string `json:"display_name"`
	StaffCode   string `json:"staff_code"`
}

func (input *StaffInput) normalize() bool {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.StaffCode = strings.TrimSpace(input.StaffCode)
	return validProfileText(input.DisplayName, 150) && validProfileText(input.StaffCode, 40)
}

type StaffRecord struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	StaffCode   string    `json:"staff_code"`
}

type StaffPage struct {
	Staff      []StaffRecord `json:"staff"`
	NextCursor *uuid.UUID    `json:"next_cursor,omitempty"`
}

func (r *Repository) ListStaff(ctx context.Context, slug string, actor uuid.UUID, after *uuid.UUID) (StaffPage, error) {
	const limit = 50
	var list StaffPage
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(page) ORDER BY page.id)
		                FILTER (WHERE page.id IS NOT NULL), '[]'::jsonb)
		FROM tenants t
		JOIN users u ON u.id=$2 AND u.status='ACTIVE'
		LEFT JOIN LATERAL (
			SELECT id,user_id,display_name,staff_code FROM staff_profiles
			WHERE tenant_id=t.id AND ($3::uuid IS NULL OR id>$3)
			ORDER BY id LIMIT $4
		) page ON true
		WHERE t.slug=$1::citext AND t.status='ACTIVE'
		  AND EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id=t.id AND m.user_id=u.id
		      AND m.status='ACTIVE' AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL'))
		GROUP BY t.id
	`, slug, actor, after, limit+1).Scan(&list.Staff)
	if err != nil {
		return StaffPage{}, scopeError(err)
	}
	if len(list.Staff) > limit {
		list.Staff = list.Staff[:limit]
		last := list.Staff[limit-1].ID
		list.NextCursor = &last
	}
	return list, nil
}

func (r *Repository) CreateStaff(ctx context.Context, slug string, actor, membershipID uuid.UUID, input StaffInput, requestID string) (StaffRecord, error) {
	return r.saveStaff(ctx, slug, actor, uuid.New(), &membershipID, input, requestID)
}

func (r *Repository) UpdateStaff(ctx context.Context, slug string, actor, staffID uuid.UUID, input StaffInput, requestID string) (StaffRecord, error) {
	return r.saveStaff(ctx, slug, actor, staffID, nil, input, requestID)
}

func (r *Repository) saveStaff(ctx context.Context, slug string, actor, staffID uuid.UUID, membershipID *uuid.UUID, input StaffInput, requestID string) (StaffRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return StaffRecord{}, err
	}
	defer tx.Rollback(ctx)
	tenant, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return StaffRecord{}, err
	}

	args := []any{tenant, staffID, input.DisplayName, input.StaffCode}
	query := `UPDATE staff_profiles SET display_name=$3,staff_code=$4,updated_at=clock_timestamp()
	          WHERE tenant_id=$1 AND id=$2`
	action := "STAFF_PROFILE_UPDATED"
	if membershipID != nil {
		query = `INSERT INTO staff_profiles(tenant_id,id,display_name,staff_code,user_id,membership_role)
		        SELECT $1,$2,$3,$4,m.user_id,m.role FROM memberships m
		        JOIN users u ON u.id=m.user_id AND u.status='ACTIVE'
		        WHERE m.tenant_id=$1 AND m.id=$5 AND m.status='ACTIVE'
		          AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL','TEACHER','BURSAR')
		        FOR SHARE OF m`
		args = append(args, *membershipID)
		action = "STAFF_PROFILE_CREATED"
	}
	var record StaffRecord
	err = tx.QueryRow(ctx, query+" RETURNING id,user_id,display_name,staff_code", args...).
		Scan(&record.ID, &record.UserID, &record.DisplayName, &record.StaffCode)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			(pgErr.ConstraintName == "staff_profiles_tenant_staff_code_unique" ||
				pgErr.ConstraintName == "staff_profiles_tenant_user_unique") {
			return StaffRecord{}, ErrStaffProfileConflict
		}
		return StaffRecord{}, scopeError(err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
		VALUES ($1,$2,$3,$4,'staff_profile',$5,$6)
	`, uuid.New(), tenant, actor, action, staffID.String(), requestID)
	if err != nil {
		return StaffRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StaffRecord{}, err
	}
	return record, nil
}

type StaffCandidate struct {
	MembershipID uuid.UUID `json:"membership_id"`
	UserID       uuid.UUID `json:"user_id"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	AccountEmail string    `json:"account_email"`
}

type StaffCandidatePage struct {
	Candidates []StaffCandidate `json:"candidates"`
	NextCursor *uuid.UUID       `json:"next_cursor,omitempty"`
}

func (r *Repository) ListStaffCandidates(ctx context.Context, slug string, actor uuid.UUID, after *uuid.UUID) (StaffCandidatePage, error) {
	const limit = 50
	var result StaffCandidatePage
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(page) ORDER BY page.user_id)
		                FILTER (WHERE page.user_id IS NOT NULL),'[]'::jsonb)
		FROM tenants t
		JOIN users actor ON actor.id=$2 AND actor.status='ACTIVE'
		LEFT JOIN LATERAL (
			SELECT DISTINCT ON (m.user_id)
			       m.id AS membership_id,m.user_id,u.full_name AS display_name,u.email AS account_email,m.role
			FROM memberships m
			JOIN users u ON u.id=m.user_id AND u.status='ACTIVE'
			WHERE m.tenant_id=t.id AND m.status='ACTIVE' AND ($3::uuid IS NULL OR m.user_id>$3)
			  AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL','TEACHER','BURSAR')
			  AND NOT EXISTS (SELECT 1 FROM staff_profiles p WHERE p.tenant_id=t.id AND p.user_id=m.user_id)
			ORDER BY m.user_id,m.id LIMIT $4
		) page ON true
		WHERE t.slug=$1::citext AND t.status='ACTIVE'
		  AND EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id=t.id AND m.user_id=actor.id
		      AND m.status='ACTIVE' AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL'))
		GROUP BY t.id
	`, slug, actor, after, limit+1).Scan(&result.Candidates)
	if err != nil {
		return StaffCandidatePage{}, scopeError(err)
	}
	if len(result.Candidates) > limit {
		result.Candidates = result.Candidates[:limit]
		last := result.Candidates[limit-1].UserID
		result.NextCursor = &last
	}
	return result, nil
}

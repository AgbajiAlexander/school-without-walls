package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type postgresRepository struct {
	pool        *pgxpool.Pool
	staffLimits config.StaffInvitationLimits
}

func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return NewPostgresRepositoryWithStaffLimits(pool, config.DefaultStaffInvitationLimits())
}

func NewPostgresRepositoryWithStaffLimits(pool *pgxpool.Pool, limits config.StaffInvitationLimits) Repository {
	if limits == (config.StaffInvitationLimits{}) {
		limits = config.DefaultStaffInvitationLimits()
	}
	return &postgresRepository{pool: pool, staffLimits: limits}
}

func (r *postgresRepository) IsPlatformAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM platform_admins
			WHERE user_id = $1
		)
	`, userID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check platform administrator: %w", err)
	}
	return allowed, nil
}

func (r *postgresRepository) Provision(ctx context.Context, tenant *Tenant, invitation *auth.Invitation, event *auth.AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant provisioning: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status, locale, timezone, branding, feature_flags, education_stages, created_at, updated_at)
		VALUES ($1, $2, $3, 'PENDING_SETUP', $4, $5, '{}'::jsonb, '{}'::jsonb, $6, $7, $7)
	`, tenant.ID, tenant.Slug, tenant.Name, tenant.Locale, tenant.Timezone, tenant.EducationStages, tenant.CreatedAt)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return ErrConflict
		}
		return fmt.Errorf("create tenant: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO invitations (id, tenant_id, invited_email, target_role, token_hash, status, expires_at, created_by, created_at)
		VALUES ($1, $2, $3, 'SCHOOL_OWNER', $4, 'PENDING', $5, $6, $7)
	`, invitation.ID, tenant.ID, invitation.InvitedEmail, invitation.TokenHash, invitation.ExpiresAt, invitation.CreatedBy, invitation.CreatedAt)
	if err != nil {
		return fmt.Errorf("create owner invitation: %w", err)
	}
	if err := writeAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant provisioning: %w", err)
	}
	return nil
}

func (r *postgresRepository) CancelProvision(ctx context.Context, tenantID, invitationID uuid.UUID, event *auth.AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant provisioning rollback: %w", err)
	}
	defer tx.Rollback(ctx)

	command, err := tx.Exec(ctx, `
		UPDATE invitations SET status = 'REVOKED'
		WHERE id = $1 AND tenant_id = $2 AND status = 'PENDING'
	`, invitationID, tenantID)
	if err != nil {
		return fmt.Errorf("revoke undelivered owner invitation: %w", err)
	}
	if command.RowsAffected() != 1 {
		return errors.New("undelivered owner invitation was not pending")
	}
	command, err = tx.Exec(ctx, `UPDATE tenants SET status = 'ARCHIVED' WHERE id = $1 AND status = 'PENDING_SETUP'`, tenantID)
	if err != nil {
		return fmt.Errorf("archive undelivered tenant: %w", err)
	}
	if command.RowsAffected() != 1 {
		return errors.New("unprovisioned tenant was not pending setup")
	}
	event.TenantID = &tenantID
	if err := writeAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant provisioning rollback: %w", err)
	}
	return nil
}

func (r *postgresRepository) List(ctx context.Context) ([]Tenant, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, slug::text, name, status::text, locale, timezone, branding, feature_flags, education_stages, created_at, updated_at
		FROM tenants
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()
	tenants := make([]Tenant, 0)
	for rows.Next() {
		tenant, err := scanTenant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, *tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants: %w", err)
	}
	return tenants, nil
}

const memberSchoolQuery = `
	SELECT t.id, t.slug::text, t.name, t.status::text, t.locale, t.timezone,
		t.branding, t.feature_flags, t.education_stages, t.created_at, t.updated_at,
		MIN(m.role), BOOL_OR(m.role = 'SCHOOL_OWNER'),
		BOOL_OR(m.role IN ('SCHOOL_OWNER', 'SCHOOL_ADMIN', 'PRINCIPAL')), BOOL_OR(m.role = 'STUDENT'), BOOL_OR(m.role = 'TEACHER'), BOOL_OR(m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN'))
	FROM tenants t
	JOIN memberships m ON m.tenant_id = t.id
	JOIN users u ON u.id = m.user_id
	WHERE m.user_id = $1 AND m.status = 'ACTIVE' AND t.status = 'ACTIVE' AND u.status = 'ACTIVE'
`

func (r *postgresRepository) GetForMember(ctx context.Context, slug string, userID uuid.UUID) (*Tenant, error) {
	row := r.pool.QueryRow(ctx, memberSchoolQuery+` AND t.slug = $2 GROUP BY t.id`, userID, slug)
	tenant, err := scanTenantWithRole(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant for active member: %w", err)
	}
	return tenant, nil
}

func (r *postgresRepository) UpdateSettings(ctx context.Context, slug string, userID uuid.UUID, request *SettingsRequest, event *auth.AuditEvent) (*Tenant, error) {
	branding, err := json.Marshal(request.Branding)
	if err != nil {
		return nil, fmt.Errorf("encode tenant branding: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tenant settings update: %w", err)
	}
	defer tx.Rollback(ctx)

	var tenantID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE tenants t SET name = $1, locale = $2, timezone = $3,
			branding = $4::jsonb, education_stages = $5,
			updated_at = CURRENT_TIMESTAMP
		FROM memberships m
		WHERE t.slug = $6::citext AND t.id = m.tenant_id AND m.user_id = $7
			AND m.status = 'ACTIVE' AND m.role IN ('SCHOOL_OWNER', 'SCHOOL_ADMIN')
			AND t.status = 'ACTIVE'
		RETURNING t.id
	`, request.Name, request.Locale, request.Timezone, branding, request.EducationStages, slug, userID).Scan(&tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant settings: %w", err)
	}
	event.TenantID = &tenantID
	event.SubjectID = tenantID.String()
	if err := writeAuditEvent(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tenant settings update: %w", err)
	}
	return r.GetForMember(ctx, slug, userID)
}

type rowScanner interface {
	Scan(...any) error
}

func scanTenant(row rowScanner) (*Tenant, error) {
	var tenant Tenant
	var branding, flags []byte
	err := row.Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.Status, &tenant.Locale,
		&tenant.Timezone, &branding, &flags, &tenant.EducationStages,
		&tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(branding, &tenant.Branding); err != nil {
		return nil, fmt.Errorf("decode tenant branding: %w", err)
	}
	if err := json.Unmarshal(flags, &tenant.FeatureFlags); err != nil {
		return nil, fmt.Errorf("decode tenant feature flags: %w", err)
	}
	return &tenant, nil
}

func scanTenantWithRole(row rowScanner) (*Tenant, error) {
	var tenant Tenant
	var branding, flags []byte
	err := row.Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.Status, &tenant.Locale,
		&tenant.Timezone, &branding, &flags, &tenant.EducationStages,
		&tenant.CreatedAt, &tenant.UpdatedAt, &tenant.CurrentRole, &tenant.CanInviteStaff, &tenant.CanManageAcademics, &tenant.CanReadLessons, &tenant.CanTeachLessons, &tenant.CanManageLessonReview,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(branding, &tenant.Branding); err != nil {
		return nil, fmt.Errorf("decode tenant branding: %w", err)
	}
	if err := json.Unmarshal(flags, &tenant.FeatureFlags); err != nil {
		return nil, fmt.Errorf("decode tenant feature flags: %w", err)
	}
	return &tenant, nil
}

func writeAuditEvent(ctx context.Context, tx pgx.Tx, event *auth.AuditEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	metadata, err := json.Marshal(event.MetadataRedacted)
	if err != nil {
		return fmt.Errorf("encode tenant audit metadata: %w", err)
	}
	var createdAt *time.Time
	if !event.CreatedAt.IsZero() {
		createdAt = &event.CreatedAt
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor_id, action, subject_type, subject_id, request_id, metadata_redacted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9::timestamptz, CURRENT_TIMESTAMP))
	`, event.ID, event.TenantID, event.ActorID, event.Action, event.SubjectType, event.SubjectID, event.RequestID, metadata, createdAt)
	if err != nil {
		return fmt.Errorf("write tenant audit event: %w", err)
	}
	return nil
}

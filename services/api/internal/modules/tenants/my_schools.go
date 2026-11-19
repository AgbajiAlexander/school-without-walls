package tenants

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

// ListForMember returns only active schools with an active membership.
func (r *postgresRepository) ListForMember(
	ctx context.Context,
	userID uuid.UUID,
) ([]Tenant, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT t.id, t.slug::text, t.name, t.status::text,
               t.locale, t.timezone, t.branding, t.feature_flags,
               t.education_stages, t.created_at, t.updated_at,
               m.role
        FROM tenants t
        JOIN memberships m ON m.tenant_id = t.id
        WHERE m.user_id = $1
          AND m.status = 'ACTIVE'
          AND t.status = 'ACTIVE'
        ORDER BY t.name, t.id
    `, userID)
	if err != nil {
		return nil, fmt.Errorf("list member schools: %w", err)
	}
	defer rows.Close()

	result := make([]Tenant, 0)
	for rows.Next() {
		tenant, err := scanTenantWithRole(rows)
		if err != nil {
			return nil, fmt.Errorf("scan member school: %w", err)
		}
		result = append(result, *tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate member schools: %w", err)
	}
	return result, nil
}

// ListForMember implements the repository contract for unit tests.
func (m *MockRepository) ListForMember(
	ctx context.Context,
	userID uuid.UUID,
) ([]Tenant, error) {
	all, err := m.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Tenant, 0)
	for _, tenant := range all {
		role, member := m.Memberships[membershipKey(tenant.Slug, userID)]
		if !member || tenant.Status != "ACTIVE" {
			continue
		}
		tenant.CurrentRole = role
		result = append(result, tenant)
	}
	return result, nil
}

// ListMine does not require platform-administrator privileges.
func (s *Service) ListMine(
	ctx context.Context,
	actorID uuid.UUID,
) ([]Tenant, error) {
	return s.repo.ListForMember(ctx, actorID)
}

// ListMine resolves the identity from authenticated middleware.
// It never accepts a user ID supplied by the browser.
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}

	schools, err := h.service.ListMine(r.Context(), actor.ID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	apphttp.JSON(w, r, http.StatusOK, map[string]any{
		"schools": schools,
	})
}

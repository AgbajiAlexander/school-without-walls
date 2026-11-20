package tenants

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type MockRepository struct {
	mu             sync.RWMutex
	PlatformAdmins map[uuid.UUID]bool
	Tenants        map[string]*Tenant
	Memberships    map[string]string
	Invitations    map[uuid.UUID]auth.Invitation
	AuditEvents    []*auth.AuditEvent
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		PlatformAdmins: make(map[uuid.UUID]bool),
		Tenants:        make(map[string]*Tenant),
		Memberships:    make(map[string]string),
		Invitations:    make(map[uuid.UUID]auth.Invitation),
	}
}

func (m *MockRepository) IsPlatformAdmin(_ context.Context, userID uuid.UUID) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.PlatformAdmins[userID], nil
}

func (m *MockRepository) Provision(_ context.Context, tenant *Tenant, invitation *auth.Invitation, event *auth.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.Tenants[tenant.Slug]; exists {
		return ErrConflict
	}
	invitation.TenantSlug = tenant.Slug
	invitation.TenantName = tenant.Name
	m.Tenants[tenant.Slug] = cloneTenant(tenant)
	m.Invitations[invitation.ID] = *invitation
	m.AuditEvents = append(m.AuditEvents, event)
	return nil
}

func (m *MockRepository) CancelProvision(_ context.Context, tenantID, invitationID uuid.UUID, event *auth.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	invitation, exists := m.Invitations[invitationID]
	if !exists || invitation.Status != auth.InvitationStatusPending {
		return errors.New("owner invitation was not pending")
	}
	invitation.Status = auth.InvitationStatusRevoked
	m.Invitations[invitationID] = invitation
	for _, tenant := range m.Tenants {
		if tenant.ID == tenantID && tenant.Status == "PENDING_SETUP" {
			tenant.Status = "ARCHIVED"
			m.AuditEvents = append(m.AuditEvents, event)
			return nil
		}
	}
	return ErrNotFound
}

func (m *MockRepository) List(_ context.Context) ([]Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]Tenant, 0, len(m.Tenants))
	for _, tenant := range m.Tenants {
		list = append(list, *cloneTenant(tenant))
	}
	return list, nil
}

func (m *MockRepository) GetForMember(_ context.Context, slug string, userID uuid.UUID) (*Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tenant, exists := m.Tenants[slug]
	if !exists || tenant.Status != "ACTIVE" {
		return nil, ErrNotFound
	}
	role, member := m.Memberships[membershipKey(slug, userID)]
	if !member {
		return nil, ErrNotFound
	}
	result := cloneTenant(tenant)
	result.CurrentRole = role
	result.CanInviteStaff = role == "SCHOOL_OWNER"
	result.CanManageAcademics = mockCanManageAcademics(role)
	return result, nil
}

func (m *MockRepository) UpdateSettings(_ context.Context, slug string, userID uuid.UUID, request *SettingsRequest, event *auth.AuditEvent) (*Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tenant, exists := m.Tenants[slug]
	if !exists || tenant.Status != "ACTIVE" {
		return nil, ErrNotFound
	}
	role, member := m.Memberships[membershipKey(slug, userID)]
	if !member || (role != "SCHOOL_OWNER" && role != "SCHOOL_ADMIN") {
		return nil, ErrForbidden
	}
	tenant.Name = request.Name
	tenant.Locale = request.Locale
	tenant.Timezone = request.Timezone
	tenant.Branding = request.Branding
	tenant.EducationStages = append([]string(nil), request.EducationStages...)
	m.AuditEvents = append(m.AuditEvents, event)
	result := cloneTenant(tenant)
	result.CurrentRole = role
	result.CanInviteStaff = role == "SCHOOL_OWNER"
	result.CanManageAcademics = mockCanManageAcademics(role)
	return result, nil
}

func mockCanManageAcademics(role string) bool {
	return role == "SCHOOL_OWNER" || role == "SCHOOL_ADMIN" || role == "PRINCIPAL"
}

func (m *MockRepository) AddMembership(slug string, userID uuid.UUID, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Memberships[membershipKey(slug, userID)] = role
}

func membershipKey(slug string, userID uuid.UUID) string {
	return slug + ":" + userID.String()
}

func cloneTenant(tenant *Tenant) *Tenant {
	copy := *tenant
	copy.EducationStages = append([]string(nil), tenant.EducationStages...)
	copy.Branding = make(map[string]any, len(tenant.Branding))
	for key, value := range tenant.Branding {
		copy.Branding[key] = value
	}
	copy.FeatureFlags = make(map[string]bool, len(tenant.FeatureFlags))
	for key, value := range tenant.FeatureFlags {
		copy.FeatureFlags[key] = value
	}
	return &copy
}

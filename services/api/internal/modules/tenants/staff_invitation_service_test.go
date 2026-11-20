package tenants

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type staffInviteRepoStub struct {
	tenantID     uuid.UUID
	createErr    error
	revokeErr    error
	confirmErr   error
	createCalls  int
	revokeCalls  int
	confirmCalls int
	invitation   *auth.Invitation
	event        *auth.AuditEvent
}

func (r *staffInviteRepoStub) ConfirmStaffInvitationDelivery(context.Context, uuid.UUID, uuid.UUID) error {
	r.confirmCalls++
	return r.confirmErr
}

func (r *staffInviteRepoStub) CreateStaffInvitation(
	_ context.Context, _ string, _ uuid.UUID,
	inv *auth.Invitation, event *auth.AuditEvent,
) (string, error) {
	r.createCalls++
	if r.createErr != nil {
		return "", r.createErr
	}
	inv.TenantID = &r.tenantID
	r.invitation = inv
	r.event = event
	return "Morfol Demo School", nil
}

func (r *staffInviteRepoStub) RevokeStaffInvitation(
	_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *auth.AuditEvent,
) error {
	r.revokeCalls++
	return r.revokeErr
}

type staffInviteMailerStub struct {
	calls     int
	email     string
	token     string
	school    string
	role      string
	expiresAt time.Time
	sendErr   error
}

func (m *staffInviteMailerStub) SendStaffInvitation(
	_ context.Context,
	email, token, school, role string, expiresAt time.Time,
) error {
	m.calls++
	m.email = email
	m.token = token
	m.school = school
	m.role = role
	m.expiresAt = expiresAt
	return m.sendErr
}

func TestStaffInvitationService(t *testing.T) {
	actorID := uuid.New()

	newService := func() (
		*StaffInvitationService,
		*staffInviteRepoStub,
		*staffInviteMailerStub,
	) {
		repo := &staffInviteRepoStub{tenantID: uuid.New()}
		mailer := &staffInviteMailerStub{}
		return NewStaffInvitationService(repo, mailer), repo, mailer
	}

	request := StaffInvitationRequest{
		Email: " ADMIN@SCHOOL.EXAMPLE ",
		Role:  "SCHOOL_ADMIN",
	}

	t.Run("successful invitation", func(t *testing.T) {
		service, repo, mailer := newService()

		result, err := service.InviteStaff(
			context.Background(), actorID,
			"morfol-demo", request, "test-success",
		)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || result.Email != "admin@school.example" ||
			result.Role != "SCHOOL_ADMIN" {
			t.Fatalf("unexpected result: %+v", result)
		}
		if repo.createCalls != 1 || mailer.calls != 1 {
			t.Fatalf("create=%d mail=%d", repo.createCalls, mailer.calls)
		}
		if mailer.school != "Morfol Demo School" ||
			mailer.email != result.Email ||
			mailer.role != result.Role ||
			mailer.token == "" {
			t.Fatal("incorrect staff invitation delivery")
		}
		if repo.invitation.TokenHash != auth.HashToken(mailer.token) {
			t.Fatal("stored token hash does not match delivered token")
		}
		if repo.invitation.TokenHash == mailer.token {
			t.Fatal("raw token was stored")
		}
		if repo.event.Action != "tenant.staff_invitation.create" {
			t.Fatal("invitation audit action is missing")
		}
		if result.ExpiresAt.Before(time.Now().Add(6 * 24 * time.Hour)) {
			t.Fatal("invitation expiry is unexpectedly short")
		}
		if repo.revokeCalls != 0 || repo.confirmCalls != 1 {
			t.Fatal("expected successful delivery to be confirmed exactly once")
		}
	})

	t.Run("missing actor is denied", func(t *testing.T) {
		service, repo, mailer := newService()
		_, err := service.InviteStaff(
			context.Background(), uuid.Nil,
			"morfol-demo", request, "no-actor",
		)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected forbidden, got %v", err)
		}
		if repo.createCalls != 0 || mailer.calls != 0 {
			t.Fatal("unauthorized request performed a side effect")
		}
	})

	t.Run("forbidden role is rejected before database access", func(t *testing.T) {
		service, repo, mailer := newService()
		_, err := service.InviteStaff(
			context.Background(), actorID,
			"morfol-demo",
			StaffInvitationRequest{
				Email: "owner@school.example",
				Role:  "SCHOOL_OWNER",
			},
			"invalid-role",
		)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
		if repo.createCalls != 0 || mailer.calls != 0 {
			t.Fatal("invalid request performed a side effect")
		}
	})

	t.Run("repository refusal prevents delivery", func(t *testing.T) {
		service, repo, mailer := newService()
		repo.createErr = ErrForbidden

		_, err := service.InviteStaff(
			context.Background(), actorID,
			"morfol-demo", request, "repo-denied",
		)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected forbidden, got %v", err)
		}
		if mailer.calls != 0 {
			t.Fatal("email sent despite repository rejection")
		}
	})

	t.Run("quota rejection never calls the mailer", func(t *testing.T) {
		service, repo, mailer := newService()
		repo.createErr = &staffInvitationRateLimitError{retryAfter: time.Minute}
		result, err := service.InviteStaff(context.Background(), actorID, "morfol-demo", request, "quota")
		var limited *staffInvitationRateLimitError
		if result != nil || !errors.As(err, &limited) || mailer.calls != 0 || repo.confirmCalls != 0 || repo.revokeCalls != 0 {
			t.Fatal("quota rejection must return before delivery")
		}
	})
	t.Run("delivery failure revokes invitation", func(t *testing.T) {
		service, repo, mailer := newService()
		mailer.sendErr = errors.New("mail delivery failure")

		result, err := service.InviteStaff(
			context.Background(), actorID,
			"morfol-demo", request, "delivery-failure",
		)
		if err == nil || result != nil {
			t.Fatalf("expected failure without result: %+v %v", result, err)
		}
		if repo.revokeCalls != 1 || repo.confirmCalls != 0 {
			t.Fatal("expected one revocation and no confirmation after delivery failure")
		}
	})

	t.Run("revocation failure is reported", func(t *testing.T) {
		service, repo, mailer := newService()
		mailer.sendErr = errors.New("mail delivery failure")
		repo.revokeErr = errors.New("revocation failure")

		_, err := service.InviteStaff(
			context.Background(), actorID,
			"morfol-demo", request, "revoke-failure",
		)
		if err == nil ||
			!errors.Is(err, repo.revokeErr) ||
			!errors.Is(err, mailer.sendErr) {
			t.Fatalf("expected both errors, got %v", err)
		}
	})

	t.Run("confirmation failure returns no success", func(t *testing.T) {
		service, repo, mailer := newService()
		repo.confirmErr = errors.New("database confirmation failure")
		result, err := service.InviteStaff(context.Background(), actorID, "morfol-demo", request, "confirm-failure")
		if result != nil || !errors.Is(err, repo.confirmErr) || mailer.calls != 1 || repo.confirmCalls != 1 || repo.revokeCalls != 0 {
			t.Fatal("confirmation failure was not surfaced as an unconfirmed handoff")
		}
	})
}

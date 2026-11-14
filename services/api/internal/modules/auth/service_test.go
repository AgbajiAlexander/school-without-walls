package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAuthServiceLogin(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)
	ctx := context.Background()

	rawPassword := "SecureP@ssw0rd123!"
	pwdHash, _ := HashPassword(rawPassword)

	user := &User{
		ID:           uuid.New(),
		Email:        "teacher@school.ng",
		PasswordHash: pwdHash,
		FullName:     "Teacher One",
		Status:       UserStatusActive,
	}
	_ = repo.CreateUser(ctx, user)

	// 1. Success login
	loggedUser, sessToken, csrfToken, err := svc.Login(ctx, "teacher@school.ng", rawPassword, "127.0.0.1", "test-agent", "req-1")
	if err != nil {
		t.Fatalf("expected login success, got %v", err)
	}
	if loggedUser.Email != "teacher@school.ng" {
		t.Errorf("expected email teacher@school.ng, got %s", loggedUser.Email)
	}
	if sessToken == "" || csrfToken == "" {
		t.Error("expected non-empty session and csrf tokens")
	}

	// Verify session in repo
	sess, fetchedUser, err := svc.ValidateSession(ctx, sessToken)
	if err != nil {
		t.Fatalf("failed to validate session: %v", err)
	}
	if fetchedUser.ID != user.ID {
		t.Errorf("expected user ID %v, got %v", user.ID, fetchedUser.ID)
	}

	// Verify CSRF token
	if !svc.VerifyCSRF(sess, csrfToken) {
		t.Error("expected valid CSRF token verification")
	}
	if svc.VerifyCSRF(sess, "invalid-csrf-token") {
		t.Error("expected invalid CSRF token to fail verification")
	}

	// 2. Wrong password
	_, _, _, err = svc.Login(ctx, "teacher@school.ng", "WrongPassword999!", "127.0.0.1", "test-agent", "req-2")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}

	// 3. Unknown email
	_, _, _, err = svc.Login(ctx, "nonexistent@school.ng", rawPassword, "127.0.0.1", "test-agent", "req-3")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials for unknown user, got %v", err)
	}

	// 4. Suspended user
	suspendedUser := &User{
		ID:           uuid.New(),
		Email:        "suspended@school.ng",
		PasswordHash: pwdHash,
		FullName:     "Suspended User",
		Status:       UserStatusSuspended,
	}
	_ = repo.CreateUser(ctx, suspendedUser)

	_, _, _, err = svc.Login(ctx, "suspended@school.ng", rawPassword, "127.0.0.1", "test-agent", "req-4")
	if err != ErrAccountSuspended {
		t.Errorf("expected ErrAccountSuspended, got %v", err)
	}
}

func TestInvitationAcceptanceCreatesMembershipFromStoredInvitation(t *testing.T) {
	repo := NewMockRepository()
	service := NewService(repo)
	tenantID := uuid.New()
	rawToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(context.Background(), &Invitation{
		TenantID: &tenantID, InvitedEmail: "owner@school.invalid", TargetRole: "SCHOOL_OWNER",
		TokenHash: HashToken(rawToken), Status: InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.AcceptInvitation(context.Background(), rawToken, "Fictional Owner",
		"a strong fictional test password", "127.0.0.1", "test-agent", "request"); err != nil {
		t.Fatalf("accept tenant owner invitation: %v", err)
	}
	if len(repo.Memberships) != 1 {
		t.Fatalf("expected exactly one membership, got %d", len(repo.Memberships))
	}
	membership := repo.Memberships[0]
	if membership.TenantID != tenantID || membership.Role != "SCHOOL_OWNER" || membership.Status != "ACTIVE" {
		t.Fatalf("membership must derive role and tenant from invitation: %+v", membership)
	}
	if len(repo.AuditEvents) == 0 || repo.AuditEvents[len(repo.AuditEvents)-1].TenantID == nil ||
		*repo.AuditEvents[len(repo.AuditEvents)-1].TenantID != tenantID {
		t.Fatal("tenant invitation acceptance should produce a tenant-scoped audit event")
	}
}

func TestAcceptInvitationValidatesFullNameWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name     string
		fullName string
	}{
		{name: "empty"},
		{name: "whitespace", fullName: " \t\u00a0 "},
		{name: "overlength unicode", fullName: strings.Repeat("é", 151)},
		{name: "control character", fullName: "Fictional\nUser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			service := NewService(repo)
			rawToken, err := GenerateSecureToken(32)
			if err != nil {
				t.Fatal(err)
			}
			invitation := &Invitation{
				ID: uuid.New(), InvitedEmail: "fictional-invitee@school.invalid",
				TokenHash: HashToken(rawToken), Status: InvitationStatusPending,
				ExpiresAt: time.Now().Add(time.Hour),
			}
			if err := repo.CreateInvitation(context.Background(), invitation); err != nil {
				t.Fatal(err)
			}

			_, _, _, err = service.AcceptInvitation(context.Background(), rawToken, tt.fullName,
				"a strong fictional test password", "127.0.0.1", "test-agent", "invalid-name")
			if !errors.Is(err, ErrInvalidFullName) {
				t.Fatalf("expected typed invalid-full-name error, got %v", err)
			}
			if len(repo.Users) != 0 || len(repo.Memberships) != 0 || len(repo.Sessions) != 0 {
				t.Fatalf("invalid invitation input caused side effects: users=%d memberships=%d sessions=%d",
					len(repo.Users), len(repo.Memberships), len(repo.Sessions))
			}
			if invitation.Status != InvitationStatusPending {
				t.Fatal("invalid invitation input consumed the invitation")
			}
		})
	}
}

func TestExistingAccountAcceptsTenantInvitationWithoutCredentialChanges(t *testing.T) {
	repo := NewMockRepository()
	service := NewService(repo)
	ctx := context.Background()
	password := "existing account passphrase for test"
	passwordHash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	existingUser := &User{
		ID: uuid.New(), Email: "existing-owner@school.invalid",
		PasswordHash: passwordHash, FullName: "Fictional Existing Owner", Status: UserStatusActive,
	}
	if err := repo.CreateUser(ctx, existingUser); err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	rawToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(ctx, &Invitation{
		TenantID: &tenantID, InvitedEmail: existingUser.Email, TargetRole: "SCHOOL_OWNER",
		TokenHash: HashToken(rawToken), Status: InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.AcceptInvitation(ctx, rawToken, "Ignored Proposed Name",
		"incorrect account passphrase", "127.0.0.1", "test-agent", "bad-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("existing account password must be verified, got %v", err)
	}
	accepted, _, _, err := service.AcceptInvitation(ctx, rawToken, "Ignored Proposed Name",
		password, "127.0.0.1", "test-agent", "valid-password")
	if err != nil {
		t.Fatalf("existing account owner should accept its invitation: %v", err)
	}
	if accepted.ID != existingUser.ID || accepted.FullName != "Fictional Existing Owner" || accepted.PasswordHash != passwordHash {
		t.Fatal("invitation acceptance must preserve the existing global account identity and credentials")
	}
	if len(repo.Memberships) != 1 || repo.Memberships[0].TenantID != tenantID || repo.Memberships[0].Role != "SCHOOL_OWNER" {
		t.Fatalf("expected membership from stored invitation, got %+v", repo.Memberships)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	repo := NewMockRepository()
	service := NewService(repo)
	ctx := context.Background()
	user := &User{
		ID: uuid.New(), Email: "expired-session@school.invalid",
		FullName: "Fictional Test User", Status: UserStatusActive,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	rawToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateSession(ctx, &Session{
		ID: uuid.New(), UserID: user.ID, TokenHash: HashToken(rawToken),
		CSRFTokenHash: HashToken("test-csrf"), ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ValidateSession(ctx, rawToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected expired session to be rejected, got %v", err)
	}
}

func TestAuthServiceLogout(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)
	ctx := context.Background()

	pwdHash, _ := HashPassword("SecureP@ssw0rd123!")
	user := &User{
		ID:           uuid.New(),
		Email:        "user@school.ng",
		PasswordHash: pwdHash,
		FullName:     "User One",
		Status:       UserStatusActive,
	}
	_ = repo.CreateUser(ctx, user)

	_, sessToken, _, _ := svc.Login(ctx, "user@school.ng", "SecureP@ssw0rd123!", "127.0.0.1", "agent", "req-1")
	sess, _, _ := svc.ValidateSession(ctx, sessToken)

	// Logout
	err := svc.Logout(ctx, sess.ID, user.ID, "req-2")
	if err != nil {
		t.Fatalf("unexpected logout error: %v", err)
	}

	// Validating revoked session must fail
	_, _, err = svc.ValidateSession(ctx, sessToken)
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound for revoked session, got %v", err)
	}
}

func TestAuthServiceInvitations(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)
	ctx := context.Background()

	rawToken, _ := GenerateSecureToken(32)
	inv := &Invitation{
		ID:           uuid.New(),
		InvitedEmail: "newstaff@school.ng",
		TargetRole:   "TEACHER",
		TokenHash:    HashToken(rawToken),
		Status:       InvitationStatusPending,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	_ = repo.CreateInvitation(ctx, inv)

	// Verify invitation
	verified, err := svc.VerifyInvitation(ctx, rawToken)
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}
	if verified.InvitedEmail != "newstaff@school.ng" {
		t.Errorf("expected email newstaff@school.ng, got %s", verified.InvitedEmail)
	}

	// Accept invitation
	user, sessToken, _, err := svc.AcceptInvitation(ctx, rawToken, "New Staff", "StrongStaffP@ss123!", "127.0.0.1", "agent", "req-1")
	if err != nil {
		t.Fatalf("failed to accept invitation: %v", err)
	}

	if user.Status != UserStatusActive {
		t.Errorf("expected ACTIVE status, got %s", user.Status)
	}
	if sessToken == "" {
		t.Error("expected active session token after accepting invitation")
	}

	// Replay invitation must fail
	_, _, _, err = svc.AcceptInvitation(ctx, rawToken, "New Staff", "StrongStaffP@ss123!", "127.0.0.1", "agent", "req-2")
	if err != ErrInvitationNotFound {
		t.Errorf("expected ErrInvitationNotFound for replayed invitation, got %v", err)
	}
}

func TestExpiredInvitationRejected(t *testing.T) {
	repo := NewMockRepository()
	service := NewService(repo)
	rawToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(context.Background(), &Invitation{
		ID: uuid.New(), InvitedEmail: "expired-invite@school.invalid",
		TokenHash: HashToken(rawToken), Status: InvitationStatusPending,
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyInvitation(context.Background(), rawToken); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("expected expired invitation to be rejected, got %v", err)
	}
}

func TestAuthServicePasswordReset(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)
	ctx := context.Background()

	oldPwd := "OldStrongP@ss123!"
	pwdHash, _ := HashPassword(oldPwd)
	user := &User{
		ID:           uuid.New(),
		Email:        "reset@school.ng",
		PasswordHash: pwdHash,
		FullName:     "Reset User",
		Status:       UserStatusActive,
	}
	_ = repo.CreateUser(ctx, user)

	// Start a session
	_, sessToken, _, _ := svc.Login(ctx, "reset@school.ng", oldPwd, "127.0.0.1", "agent", "req-1")
	_, _, err := svc.ValidateSession(ctx, sessToken)
	if err != nil {
		t.Fatal("session should be valid before password reset")
	}

	// Request password reset
	resetToken, err := svc.RequestPasswordReset(ctx, "reset@school.ng", "req-2")
	if err != nil {
		t.Fatalf("unexpected request reset error: %v", err)
	}
	if resetToken == "" {
		t.Fatal("expected reset token for existing user")
	}

	// Request reset for unknown email returns empty token, no error
	unknownToken, err := svc.RequestPasswordReset(ctx, "unknown@school.ng", "req-3")
	if err != nil || unknownToken != "" {
		t.Errorf("expected empty token and no error for unknown email, got %s, %v", unknownToken, err)
	}

	// Confirm password reset
	newPwd := "BrandNewP@ssw0rd456!"
	err = svc.ResetPassword(ctx, resetToken, newPwd, "req-4")
	if err != nil {
		t.Fatalf("failed to reset password: %v", err)
	}

	// Prior session must now be revoked
	_, _, err = svc.ValidateSession(ctx, sessToken)
	if err != ErrSessionNotFound {
		t.Errorf("expected prior session to be revoked after password change, got %v", err)
	}

	// Login with old password must fail
	_, _, _, err = svc.Login(ctx, "reset@school.ng", oldPwd, "127.0.0.1", "agent", "req-5")
	if err != ErrInvalidCredentials {
		t.Errorf("expected login with old password to fail, got %v", err)
	}

	// Login with new password succeeds
	_, newSess, _, err := svc.Login(ctx, "reset@school.ng", newPwd, "127.0.0.1", "agent", "req-6")
	if err != nil || newSess == "" {
		t.Fatalf("expected login with new password to succeed, got %v", err)
	}

	// Replaying reset token must fail
	err = svc.ResetPassword(ctx, resetToken, "AnotherP@ssw0rd789!", "req-7")
	if err != ErrPasswordResetNotFound {
		t.Errorf("expected ErrPasswordResetNotFound on token replay, got %v", err)
	}
}

func TestExpiredPasswordResetRejected(t *testing.T) {
	repo := NewMockRepository()
	service := NewService(repo)
	ctx := context.Background()
	oldHash, err := HashPassword("existing account secure passphrase")
	if err != nil {
		t.Fatal(err)
	}
	user := &User{
		ID: uuid.New(), Email: "expired-reset@school.invalid",
		PasswordHash: oldHash, FullName: "Fictional Test User", Status: UserStatusActive,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	rawToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreatePasswordReset(ctx, &PasswordReset{
		ID: uuid.New(), UserID: user.ID, TokenHash: HashToken(rawToken),
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.ResetPassword(ctx, rawToken, "replacement secure test passphrase", "expired-reset-test"); !errors.Is(err, ErrPasswordResetNotFound) {
		t.Fatalf("expected expired reset token to be rejected, got %v", err)
	}
}

func TestAuthServiceBootstrapAdmin(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)
	ctx := context.Background()

	// Initial bootstrap succeeds
	admin, err := svc.BootstrapAdmin(ctx, "admin@schoolwithoutwalls.ng", "System Administrator", "AdminS3cureP@ss99!", "req-1")
	if err != nil {
		t.Fatalf("expected bootstrap admin to succeed, got %v", err)
	}
	if admin.Email != "admin@schoolwithoutwalls.ng" {
		t.Errorf("unexpected admin email: %s", admin.Email)
	}
	if !repo.PlatformAdmins[admin.ID] {
		t.Fatal("bootstrap must create an explicit platform-administrator grant")
	}

	// Second bootstrap attempt denied
	_, err = svc.BootstrapAdmin(ctx, "another@schoolwithoutwalls.ng", "Second Admin", "AdminS3cureP@ss99!", "req-2")
	if err != ErrAdminAlreadyExists {
		t.Errorf("expected ErrAdminAlreadyExists for second bootstrap, got %v", err)
	}
}

func TestInvitationRequiresExistingPasswordAndPreservesCredentials(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)
	ctx := context.Background()
	oldPassword := "existing account passphrase"
	passwordHash, err := HashPassword(oldPassword)
	if err != nil {
		t.Fatal(err)
	}

	existing := &User{
		ID: uuid.New(), Email: "existing@school.invalid",
		PasswordHash: passwordHash, FullName: "Existing User", Status: UserStatusActive,
	}
	if err := repo.CreateUser(ctx, existing); err != nil {
		t.Fatal(err)
	}
	rawToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	invitation := &Invitation{
		ID: uuid.New(), InvitedEmail: existing.Email, TokenHash: HashToken(rawToken),
		Status: InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.CreateInvitation(ctx, invitation); err != nil {
		t.Fatal(err)
	}

	_, _, _, err = svc.AcceptInvitation(ctx, rawToken, "Changed Name", "incorrect existing password", "", "", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected existing account password rejection, got %v", err)
	}
	accepted, _, _, err := svc.AcceptInvitation(ctx, rawToken, "Changed Name", oldPassword, "", "", "valid-password")
	if err != nil {
		t.Fatalf("existing account should accept invitation with its current password: %v", err)
	}
	if accepted.ID != existing.ID || accepted.FullName != "Existing User" || accepted.PasswordHash != passwordHash {
		t.Fatal("invitation must preserve the existing user's identity and password hash")
	}
	loggedIn, _, _, err := svc.Login(ctx, existing.Email, oldPassword, "", "", "req-login")
	if err != nil || loggedIn.ID != existing.ID {
		t.Fatalf("existing password must remain usable, got user=%v err=%v", loggedIn, err)
	}
	verified, err := svc.VerifyInvitation(ctx, rawToken)
	if !errors.Is(err, ErrInvitationNotFound) || verified != nil {
		t.Fatalf("accepted invitation must not be reusable, got invitation=%v err=%v", verified, err)
	}
}

func TestSuspendUserRevokesSessions(t *testing.T) {
	repo := NewMockRepository()
	service := NewService(repo)
	ctx := context.Background()
	password := "test account secure passphrase"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := &User{
		ID: uuid.New(), Email: "suspend@school.invalid",
		PasswordHash: hash, FullName: "Test User", Status: UserStatusActive,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	_, token, _, err := service.Login(ctx, user.Email, password, "", "", "login")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SuspendUser(ctx, user.ID, user.ID, "suspend"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ValidateSession(ctx, token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("suspended user's session should be revoked, got %v", err)
	}
	if _, _, _, err := service.Login(ctx, user.Email, password, "", "", "login-suspended"); !errors.Is(err, ErrAccountSuspended) {
		t.Fatalf("suspended account should not log in, got %v", err)
	}
}

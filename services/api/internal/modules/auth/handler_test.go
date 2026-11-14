package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

type testMailMessage struct {
	To      string
	Subject string
	Body    string
}

type testMailer struct {
	mu       sync.Mutex
	messages []testMailMessage
}

func (m *testMailer) SendPasswordReset(_ context.Context, email, token string, _ time.Time) error {
	resetURL := &url.URL{Scheme: "http", Host: "localhost", Path: "/reset-password"}
	query := resetURL.Query()
	query.Set("token", token)
	resetURL.RawQuery = query.Encode()
	m.mu.Lock()
	m.messages = append(m.messages, testMailMessage{
		To: email, Subject: "Reset password", Body: "Reset password: " + resetURL.String(),
	})
	m.mu.Unlock()
	return nil
}

func (m *testMailer) capturedMessages() []testMailMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	messages := make([]testMailMessage, len(m.messages))
	copy(messages, m.messages)
	return messages
}

func setupTestRouter(repo Repository) (http.Handler, *Service, *Handler) {
	return setupTestRouterWithMailer(repo, &testMailer{})
}

func setupTestRouterWithMailer(repo Repository, mailer Mailer) (http.Handler, *Service, *Handler) {
	svc := NewService(repo)
	handler := NewHandler(svc, false, mailer, nil)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(Authenticate(svc))

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Route("/auth", func(a chi.Router) {
			a.Post("/login", handler.Login)
			a.Post("/forgot-password", handler.ForgotPassword)
			a.Post("/reset-password", handler.ResetPassword)

			// Protected routes
			a.Group(func(protected chi.Router) {
				protected.Use(RequireAuth)
				protected.Use(RequireCSRF(svc))
				protected.Get("/me", handler.Me)
				protected.Post("/logout", handler.Logout)
			})
		})

		v1.Route("/invitations", func(inv chi.Router) {
			inv.Get("/{token}/verify", handler.VerifyInvitation)
			inv.Post("/accept", handler.AcceptInvitation)
		})
	})

	return r, svc, handler
}

func TestHandlerLoginSuccessAndMe(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)

	pwdHash, _ := HashPassword("SecureP@ssw0rd123!")
	user := &User{
		ID:           uuid.New(),
		Email:        "user@school.ng",
		PasswordHash: pwdHash,
		FullName:     "Alexander Test",
		Status:       UserStatusActive,
	}
	_ = repo.CreateUser(context.Background(), user)

	// 1. Successful Login
	loginBody, _ := json.Marshal(LoginRequest{
		Email:    "user@school.ng",
		Password: "SecureP@ssw0rd123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify HttpOnly cookie was set
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == CookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.Value == "" {
		t.Fatalf("expected valid HttpOnly session cookie, got %v", sessionCookie)
	}
	if sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("development session cookie should be non-Secure and SameSite=Lax")
	}

	var loginResp apphttp.SuccessResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &loginResp)
	dataMap := loginResp.Data.(map[string]any)
	csrfToken := dataMap["csrf_token"].(string)
	if csrfToken == "" {
		t.Fatal("expected CSRF token in login response")
	}

	// 2. Access /api/v1/auth/me with session cookie
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.AddCookie(sessionCookie)
	rrMe := httptest.NewRecorder()

	router.ServeHTTP(rrMe, reqMe)

	if rrMe.Code != http.StatusOK {
		t.Fatalf("expected /auth/me status 200, got %d", rrMe.Code)
	}

	var meResp apphttp.SuccessResponse
	_ = json.Unmarshal(rrMe.Body.Bytes(), &meResp)
	meData := meResp.Data.(map[string]any)
	userObj := meData["user"].(map[string]any)
	if userObj["email"] != "user@school.ng" {
		t.Errorf("expected email user@school.ng, got %v", userObj["email"])
	}
	csrfToken = meData["csrf_token"].(string)

	// 3. Logout with CSRF token
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	reqLogout.AddCookie(sessionCookie)
	reqLogout.Header.Set(HeaderCSRF, csrfToken)
	rrLogout := httptest.NewRecorder()

	router.ServeHTTP(rrLogout, reqLogout)

	if rrLogout.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d", rrLogout.Code)
	}

	// Check cleared cookie
	logoutCookies := rrLogout.Result().Cookies()
	var clearedCookie *http.Cookie
	for _, c := range logoutCookies {
		if c.Name == CookieName {
			clearedCookie = c
			break
		}
	}
	if clearedCookie == nil || clearedCookie.MaxAge > 0 {
		t.Fatal("expected session cookie to be cleared on logout")
	}

	// 4. Calling /auth/me after logout must now return 401
	reqMeAfter := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMeAfter.AddCookie(sessionCookie)
	rrMeAfter := httptest.NewRecorder()

	router.ServeHTTP(rrMeAfter, reqMeAfter)

	if rrMeAfter.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after logout, got %d", rrMeAfter.Code)
	}
}

func TestHandlerLoginInvalidCredentials(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)

	body, _ := json.Marshal(LoginRequest{
		Email:    "unknown@school.ng",
		Password: "RandomPassword123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unknown user, got %d", rr.Code)
	}

	var errResp apphttp.ErrorResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &errResp)
	if errResp.Error.Code != apphttp.ErrCodeUnauthenticated {
		t.Errorf("expected error code UNAUTHENTICATED, got %s", errResp.Error.Code)
	}
}

func TestHandlerLoginDoesNotDiscloseAccountStatus(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)
	passwordHash, err := HashPassword("a sufficiently long test passphrase")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []UserStatus{UserStatusPendingActivation, UserStatusSuspended} {
		user := &User{
			ID: uuid.New(), Email: string(status) + "@school.invalid",
			PasswordHash: passwordHash, FullName: "Fictional Test User", Status: status,
		}
		if err := repo.CreateUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(LoginRequest{Email: user.Email, Password: "a sufficiently long test passphrase"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "Invalid email or password") {
			t.Fatalf("account status %s should get a generic unauthorized response; got %d: %s", status, recorder.Code, recorder.Body.String())
		}
	}
}

func TestSecureAuthCookies(t *testing.T) {
	recorder := httptest.NewRecorder()
	expiresAt := time.Now().Add(time.Hour)
	setSessionCookie(recorder, true, "opaque-session", expiresAt)
	setCSRFCookie(recorder, true, "csrf-value", expiresAt)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected session and CSRF cookies, got %d", len(cookies))
	}
	sessionCookie, csrfCookie := cookies[0], cookies[1]
	if sessionCookie.Name != SecureCookieName || !sessionCookie.Secure || !sessionCookie.HttpOnly || sessionCookie.Path != "/" || sessionCookie.Domain != "" {
		t.Fatalf("session cookie is missing secure host-cookie attributes: %#v", sessionCookie)
	}
	if csrfCookie.Name != secureCSRFcookieName || !csrfCookie.Secure || csrfCookie.HttpOnly || csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("CSRF cookie is missing expected attributes: %#v", csrfCookie)
	}
}

func TestClientIPTrustsForwardingOnlyFromConfiguredProxy(t *testing.T) {
	_, trustedProxy, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	proxyNetwork := []*net.IPNet{trustedProxy}

	untrustedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	untrustedRequest.RemoteAddr = "198.51.100.10:1234"
	untrustedRequest.Header.Set("X-Forwarded-For", "203.0.113.99")
	if got := extractClientIP(untrustedRequest, proxyNetwork); got != "198.51.100.10" {
		t.Fatalf("untrusted forwarding header was used: %s", got)
	}

	trustedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	trustedRequest.RemoteAddr = "10.1.2.3:1234"
	trustedRequest.Header.Set("X-Forwarded-For", "203.0.113.4, 10.2.3.4")
	if got := extractClientIP(trustedRequest, proxyNetwork); got != "203.0.113.4" {
		t.Fatalf("expected first untrusted forwarded address, got %s", got)
	}
}

func TestHandlerRateLimiting(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)

	// Send 5 failed login attempts
	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(LoginRequest{
			Email:    "fail@school.ng",
			Password: "WrongPassword!",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.1:54321"
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d expected 401, got %d", i+1, rr.Code)
		}
	}

	// 6th attempt must be 429 RATE_LIMITED
	body, _ := json.Marshal(LoginRequest{
		Email:    "fail@school.ng",
		Password: "WrongPassword!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.1:54321"
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 6th attempt to return 429 Too Many Requests, got %d", rr.Code)
	}

	var errResp apphttp.ErrorResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &errResp)
	if errResp.Error.Code != apphttp.ErrCodeRateLimited {
		t.Errorf("expected RATE_LIMITED code, got %s", errResp.Error.Code)
	}
}

func TestHandlerInvitationAcceptance(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)

	rawToken, _ := GenerateSecureToken(32)
	inv := &Invitation{
		ID:           uuid.New(),
		InvitedEmail: "invited@school.ng",
		TargetRole:   "TEACHER",
		TokenHash:    HashToken(rawToken),
		Status:       InvitationStatusPending,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	_ = repo.CreateInvitation(context.Background(), inv)

	// Verify endpoint
	reqVerify := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/"+rawToken+"/verify", nil)
	rrVerify := httptest.NewRecorder()
	router.ServeHTTP(rrVerify, reqVerify)

	if rrVerify.Code != http.StatusOK {
		t.Fatalf("expected verify status 200, got %d: %s", rrVerify.Code, rrVerify.Body.String())
	}
	if strings.Contains(rrVerify.Body.String(), "target_role") {
		t.Fatal("invitation verification must not expose role metadata")
	}

	// Accept endpoint
	acceptBody, _ := json.Marshal(AcceptInvitationRequest{
		Token:    rawToken,
		FullName: "Invited Teacher",
		Password: "StrongTeacherP@ssword123!",
	})
	reqAccept := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewReader(acceptBody))
	reqAccept.Header.Set("Content-Type", "application/json")
	rrAccept := httptest.NewRecorder()
	router.ServeHTTP(rrAccept, reqAccept)

	if rrAccept.Code != http.StatusCreated {
		t.Fatalf("expected accept status 201, got %d: %s", rrAccept.Code, rrAccept.Body.String())
	}

	// Verify session cookie
	cookies := rrAccept.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == CookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected session cookie set upon invitation acceptance")
	}

	// Replay must fail with 422
	reqReplay := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewReader(acceptBody))
	reqReplay.Header.Set("Content-Type", "application/json")
	rrReplay := httptest.NewRecorder()
	router.ServeHTTP(rrReplay, reqReplay)

	if rrReplay.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected replay to fail with 422, got %d", rrReplay.Code)
	}
}

func TestHandlerInvitationValidationReturns422WithoutSideEffects(t *testing.T) {
	tests := []struct {
		name     string
		fullName string
	}{
		{name: "empty"},
		{name: "whitespace", fullName: " \t\u00a0 "},
		{name: "overlength unicode", fullName: strings.Repeat("é", 151)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			router, _, _ := setupTestRouter(repo)
			rawToken, err := GenerateSecureToken(32)
			if err != nil {
				t.Fatal(err)
			}
			tenantID := uuid.New()
			invitation := &Invitation{
				ID: uuid.New(), TenantID: &tenantID, InvitedEmail: "fictional-invitee@school.invalid",
				TargetRole: "TEACHER", TokenHash: HashToken(rawToken),
				Status: InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
			}
			if err := repo.CreateInvitation(context.Background(), invitation); err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(AcceptInvitationRequest{
				Token: rawToken, FullName: tt.fullName, Password: "strong fictional password",
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("invalid name must return 422, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if len(repo.Users) != 0 || len(repo.Memberships) != 0 || len(repo.Sessions) != 0 {
				t.Fatalf("invalid invitation input caused side effects: users=%d memberships=%d sessions=%d",
					len(repo.Users), len(repo.Memberships), len(repo.Sessions))
			}
			if invitation.Status != InvitationStatusPending {
				t.Fatal("invalid invitation request consumed its token")
			}
		})
	}
}

func TestHandlerInvitationInvalidAndExpiredTokensReturn422(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)
	expiredToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(context.Background(), &Invitation{
		ID: uuid.New(), InvitedEmail: "expired@school.invalid", TokenHash: HashToken(expiredToken),
		Status: InvitationStatusPending, ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	for _, token := range []string{"not-an-invitation", expiredToken} {
		body, err := json.Marshal(AcceptInvitationRequest{
			Token: token, FullName: "Fictional Invitee", Password: "strong fictional password",
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(recorder.Body.String(), "invalid, expired, or already used") {
			t.Fatalf("invalid/expired invitation must retain generic 422 behavior, got %d: %s",
				recorder.Code, recorder.Body.String())
		}
	}
	if len(repo.Users) != 0 || len(repo.Memberships) != 0 || len(repo.Sessions) != 0 {
		t.Fatalf("invalid/expired invitation caused side effects: users=%d memberships=%d sessions=%d",
			len(repo.Users), len(repo.Memberships), len(repo.Sessions))
	}
}

func TestHandlerPasswordResetFlow(t *testing.T) {
	repo := NewMockRepository()
	mailer := &testMailer{}
	router, svc, _ := setupTestRouterWithMailer(repo, mailer)
	var capturedLogs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&capturedLogs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	oldPwd := "OldPassword123!"
	pwdHash, err := HashPassword(oldPwd)
	if err != nil {
		t.Fatal(err)
	}
	user := &User{
		ID:           uuid.New(),
		Email:        "fictional-reset-user@school.invalid",
		PasswordHash: pwdHash,
		FullName:     "Fictional Reset User",
		Status:       UserStatusActive,
	}
	if err := repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}

	_, firstSession, _, err := svc.Login(context.Background(), user.Email, oldPwd, "127.0.0.1", "test-agent", "login-1")
	if err != nil {
		t.Fatal(err)
	}
	_, secondSession, _, err := svc.Login(context.Background(), user.Email, oldPwd, "127.0.0.1", "test-agent", "login-2")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Forgot password request (generic message)
	forgotBody, err := json.Marshal(ForgotPasswordRequest{Email: user.Email})
	if err != nil {
		t.Fatal(err)
	}
	reqForgot := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(forgotBody))
	reqForgot.Header.Set("Content-Type", "application/json")
	rrForgot := httptest.NewRecorder()
	router.ServeHTTP(rrForgot, reqForgot)

	if rrForgot.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for forgot password, got %d", rrForgot.Code)
	}
	if strings.Contains(rrForgot.Body.String(), "token") || strings.Contains(rrForgot.Body.String(), oldPwd) {
		t.Fatal("forgot-password response must not expose secrets")
	}
	messages := mailer.capturedMessages()
	if len(messages) != 1 || messages[0].To != user.Email {
		t.Fatalf("expected one captured reset email for the fictional account, messages=%d", len(messages))
	}
	resetURLText := strings.TrimPrefix(messages[0].Body, "Reset password: ")
	resetURL, err := url.Parse(strings.TrimSpace(resetURLText))
	if err != nil {
		t.Fatal("captured reset email did not contain a valid link")
	}
	token := resetURL.Query().Get("token")
	if token == "" || !strings.Contains(messages[0].Body, "/reset-password?") {
		t.Fatal("captured reset email must contain the one-time reset link")
	}

	// 2. Reset password endpoint
	newPassword := "BrandNewS3cureP@ssword99!"
	resetBody, err := json.Marshal(ResetPasswordRequest{
		Token:       token,
		NewPassword: newPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(resetBody))
	reqReset.Header.Set("Content-Type", "application/json")
	rrReset := httptest.NewRecorder()
	router.ServeHTTP(rrReset, reqReset)

	if rrReset.Code != http.StatusOK {
		t.Fatalf("expected reset password status 200, got %d: %s", rrReset.Code, rrReset.Body.String())
	}
	if strings.Contains(rrReset.Body.String(), token) || strings.Contains(rrReset.Body.String(), newPassword) {
		t.Fatal("reset-password response exposed a token or password")
	}
	for _, sessionToken := range []string{firstSession, secondSession} {
		if _, _, err := svc.ValidateSession(context.Background(), sessionToken); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("password reset must revoke all active sessions, got %v", err)
		}
	}

	oldLoginBody, err := json.Marshal(LoginRequest{Email: user.Email, Password: oldPwd})
	if err != nil {
		t.Fatal(err)
	}
	oldLoginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(oldLoginBody))
	oldLoginRequest.Header.Set("Content-Type", "application/json")
	oldLoginResponse := httptest.NewRecorder()
	router.ServeHTTP(oldLoginResponse, oldLoginRequest)
	if oldLoginResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old password must fail after reset, got %d", oldLoginResponse.Code)
	}

	newLoginBody, err := json.Marshal(LoginRequest{
		Email:    user.Email,
		Password: newPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(newLoginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	rrLogin := httptest.NewRecorder()
	router.ServeHTTP(rrLogin, reqLogin)
	if rrLogin.Code != http.StatusOK {
		t.Fatalf("login with new password should succeed, got %d", rrLogin.Code)
	}

	replayRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(resetBody))
	replayRequest.Header.Set("Content-Type", "application/json")
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusUnprocessableEntity || strings.Contains(replayResponse.Body.String(), token) {
		t.Fatalf("reset-token replay must be rejected without exposing the token: status=%d body=%s",
			replayResponse.Code, replayResponse.Body.String())
	}
	for _, secret := range []string{token, oldPwd, newPassword} {
		if strings.Contains(capturedLogs.String(), secret) {
			t.Fatal("password reset token or password appeared in application logs")
		}
	}
}

func TestHandlerForgotPasswordDoesNotEnumerateAccounts(t *testing.T) {
	repo := NewMockRepository()
	mailer := &testMailer{}
	router, _, _ := setupTestRouterWithMailer(repo, mailer)
	passwordHash, err := HashPassword("fictional account password phrase")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), &User{
		ID: uuid.New(), Email: "known-reset-user@school.invalid",
		PasswordHash: passwordHash, FullName: "Fictional User", Status: UserStatusActive,
	}); err != nil {
		t.Fatal(err)
	}

	messages := make([]string, 0, 2)
	for _, email := range []string{"known-reset-user@school.invalid", "missing-reset-user@school.invalid"} {
		body, err := json.Marshal(ForgotPasswordRequest{Email: email})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("forgot-password should return generic 202, got %d", response.Code)
		}
		var payload struct {
			Data struct {
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, payload.Data.Message)
	}
	if messages[0] == "" || messages[0] != messages[1] {
		t.Fatal("forgot-password response must be identical for known and unknown accounts")
	}
	if len(mailer.capturedMessages()) != 1 {
		t.Fatal("only the known fictional account should receive a captured message")
	}
}

func TestHandlerResetPasswordRejectsInvalidAndExpiredTokens(t *testing.T) {
	repo := NewMockRepository()
	router, _, _ := setupTestRouter(repo)
	passwordHash, err := HashPassword("fictional account password phrase")
	if err != nil {
		t.Fatal(err)
	}
	user := &User{
		ID: uuid.New(), Email: "expired-reset-user@school.invalid",
		PasswordHash: passwordHash, FullName: "Fictional User", Status: UserStatusActive,
	}
	if err := repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	expiredToken, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreatePasswordReset(context.Background(), &PasswordReset{
		ID: uuid.New(), UserID: user.ID, TokenHash: HashToken(expiredToken),
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	for _, token := range []string{"invalid-reset-token", expiredToken} {
		body, err := json.Marshal(ResetPasswordRequest{
			Token: token, NewPassword: "a replacement fictional password",
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(response.Body.String(), "invalid, expired, or already used") {
			t.Fatalf("invalid/expired reset token must return generic 422, got %d: %s",
				response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), token) {
			t.Fatal("reset error response must not expose the supplied token")
		}
	}
	userAfter, err := repo.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if userAfter.PasswordHash != passwordHash {
		t.Fatal("invalid or expired reset token changed the account password")
	}
}

func TestHandlerForgotPasswordRateLimit(t *testing.T) {
	repo := NewMockRepository()
	mailer := &testMailer{}
	router, _, _ := setupTestRouterWithMailer(repo, mailer)
	passwordHash, err := HashPassword("fictional rate-limited password")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), &User{
		ID: uuid.New(), Email: "rate-limited@school.invalid",
		PasswordHash: passwordHash, FullName: "Fictional User", Status: UserStatusActive,
	}); err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 4; attempt++ {
		body, err := json.Marshal(ForgotPasswordRequest{Email: "rate-limited@school.invalid"})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "198.51.100.45:1234"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if attempt <= 3 && response.Code != http.StatusAccepted {
			t.Fatalf("password reset attempt %d should be accepted, got %d", attempt, response.Code)
		}
		if attempt == 4 && response.Code != http.StatusTooManyRequests {
			t.Fatalf("fourth account reset attempt should be rate limited, got %d", response.Code)
		}
	}
	if len(mailer.capturedMessages()) != 3 {
		t.Fatalf("rate-limited request must not send email, captured=%d", len(mailer.capturedMessages()))
	}
}

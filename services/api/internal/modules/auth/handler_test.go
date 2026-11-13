package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

type testMailer struct{}

func (testMailer) SendPasswordReset(context.Context, string, string, time.Time) error {
	return nil
}

func setupTestRouter(repo Repository) (http.Handler, *Service, *Handler) {
	svc := NewService(repo)
	handler := NewHandler(svc, false, testMailer{}, nil)

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

func TestHandlerPasswordResetFlow(t *testing.T) {
	repo := NewMockRepository()
	router, svc, _ := setupTestRouter(repo)

	oldPwd := "OldPassword123!"
	pwdHash, _ := HashPassword(oldPwd)
	user := &User{
		ID:           uuid.New(),
		Email:        "student@school.ng",
		PasswordHash: pwdHash,
		FullName:     "Student Reset",
		Status:       UserStatusActive,
	}
	_ = repo.CreateUser(context.Background(), user)

	// 1. Forgot password request (generic message)
	forgotBody, _ := json.Marshal(ForgotPasswordRequest{Email: "student@school.ng"})
	reqForgot := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(forgotBody))
	reqForgot.Header.Set("Content-Type", "application/json")
	rrForgot := httptest.NewRecorder()
	router.ServeHTTP(rrForgot, reqForgot)

	if rrForgot.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for forgot password, got %d", rrForgot.Code)
	}

	// Directly generate a test token through service to test the reset-password endpoint
	token, _ := svc.RequestPasswordReset(context.Background(), "student@school.ng", "req-test")

	// 2. Reset password endpoint
	resetBody, _ := json.Marshal(ResetPasswordRequest{
		Token:       token,
		NewPassword: "BrandNewS3cureP@ssword99!",
	})
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(resetBody))
	reqReset.Header.Set("Content-Type", "application/json")
	rrReset := httptest.NewRecorder()
	router.ServeHTTP(rrReset, reqReset)

	if rrReset.Code != http.StatusOK {
		t.Fatalf("expected reset password status 200, got %d: %s", rrReset.Code, rrReset.Body.String())
	}

	// 3. Login with new password
	loginBody, _ := json.Marshal(LoginRequest{
		Email:    "student@school.ng",
		Password: "BrandNewS3cureP@ssword99!",
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	rrLogin := httptest.NewRecorder()
	router.ServeHTTP(rrLogin, reqLogin)

	if rrLogin.Code != http.StatusOK {
		t.Errorf("expected login with new password to succeed, got %d", rrLogin.Code)
	}
}

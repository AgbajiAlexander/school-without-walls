package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/modules/tenants"
)

func TestRouterEndpoints(t *testing.T) {
	cfg := &config.Config{
		Environment:        "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	mockDB := &mockPinger{err: nil}
	router := NewRouter(RouterConfig{
		Config: cfg,
		DB:     mockDB,
	})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"root health", http.MethodGet, "/health", http.StatusOK},
		{"root ready", http.MethodGet, "/ready", http.StatusOK},
		{"api v1 health", http.MethodGet, "/api/v1/health", http.StatusOK},
		{"api v1 ready", http.MethodGet, "/api/v1/ready", http.StatusOK},
		{"not found", http.MethodGet, "/api/v1/non-existent", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestRouterCORS(t *testing.T) {
	cfg := &config.Config{
		Environment:        "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	router := NewRouter(RouterConfig{
		Config: cfg,
		DB:     &mockPinger{err: nil},
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for CORS preflight, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("expected allow origin header http://localhost:3000, got %s", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestRouterNotFoundEnvelope(t *testing.T) {
	cfg := &config.Config{
		Environment:        "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	router := NewRouter(RouterConfig{
		Config: cfg,
		DB:     &mockPinger{err: nil},
	})

	req := httptest.NewRequest(http.MethodGet, "/unknown-path", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("expected error code %s, got %s", ErrCodeNotFound, resp.Error.Code)
	}
}

func TestTenantProvisionRoutesRequireVerifiedAdminAndCSRF(t *testing.T) {
	authRepo := auth.NewMockRepository()
	passwordHash, err := auth.HashPassword("a sufficiently long fictional test password")
	if err != nil {
		t.Fatal(err)
	}
	user := &auth.User{
		ID: uuid.New(), Email: "platform-admin@school.invalid",
		PasswordHash: passwordHash, FullName: "Fictional Operator", Status: auth.UserStatusActive,
	}
	if err := authRepo.CreateUser(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	tenantRepo := tenants.NewMockRepository()
	tenantRepo.PlatformAdmins[user.ID] = true
	cfg := &config.Config{
		Environment: "test", PublicWebURL: "http://localhost:3000",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}
	router := NewRouter(RouterConfig{
		Config: cfg, DB: &mockPinger{}, AuthRepo: authRepo, TenantRepo: tenantRepo,
	})
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/platform/tenants", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated tenant listing should be rejected, status=%d", unauthenticated.Code)
	}

	authService := auth.NewService(authRepo)
	_, sessionToken, csrfToken, err := authService.Login(t.Context(), user.Email, "a sufficiently long fictional test password", "127.0.0.1", "test-agent", "login")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"slug":"router-school","name":"Router School","owner_email":"owner@school.invalid"}`
	requestWithoutCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/platform/tenants", strings.NewReader(body))
	requestWithoutCSRF.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	requestWithoutCSRF.Header.Set("Origin", "http://localhost:3000")
	requestWithoutCSRF.Header.Set("Content-Type", "application/json")
	noCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(noCSRFResponse, requestWithoutCSRF)
	if noCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("authenticated provisioning without CSRF should be rejected, status=%d body=%s", noCSRFResponse.Code, noCSRFResponse.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/tenants", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.HeaderCSRF, csrfToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("valid platform admin request should create school, status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "invite/") || strings.Contains(response.Body.String(), csrfToken) {
		t.Fatal("tenant create response must not expose invitation tokens or session CSRF tokens")
	}

	captureRequest := httptest.NewRequest(http.MethodGet, "/api/v1/platform/mail-capture", nil)
	captureRequest.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	captureResponse := httptest.NewRecorder()
	router.ServeHTTP(captureResponse, captureRequest)
	if captureResponse.Code != http.StatusOK || !strings.Contains(captureResponse.Body.String(), "/invite/") {
		t.Fatalf("authorized local mail capture should contain the one-time invite link, status=%d body=%s", captureResponse.Code, captureResponse.Body.String())
	}
}

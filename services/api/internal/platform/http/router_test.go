package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/academics"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/modules/tenants"
)

func TestRouterEndpoints(t *testing.T) {
	cfg := &config.Config{
		Environment:        "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	mockDB := &mockPinger{err: nil}
	router := mustNewTestRouter(t, cfg, mockDB)

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

func TestRouterRequiresExplicitTestRepositories(t *testing.T) {
	_, err := NewRouter(RouterConfig{
		Config: &config.Config{Environment: "test"},
		DB:     &mockPinger{},
	})
	if err == nil {
		t.Fatal("test router must not construct mock repositories implicitly")
	}
}

func TestRouterRequiresAuthoritativeDatabaseOutsideTests(t *testing.T) {
	_, err := NewRouter(RouterConfig{
		Config: &config.Config{Environment: "production"},
		DB:     &mockPinger{},
	})
	if err == nil {
		t.Fatal("production router must require authoritative PostgreSQL")
	}
}

func TestRouterCORS(t *testing.T) {
	cfg := &config.Config{
		Environment:        "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	router := mustNewTestRouter(t, cfg, &mockPinger{err: nil})

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

	router := mustNewTestRouter(t, cfg, &mockPinger{err: nil})

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
	academicRepo := academics.NewMockRepository()
	tenantRepo.PlatformAdmins[user.ID] = true
	cfg := &config.Config{
		Environment: "test", PublicWebURL: "http://localhost:3000",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}
	router, err := NewRouter(RouterConfig{
		Config: cfg, DB: &mockPinger{}, AuthRepo: authRepo, TenantRepo: tenantRepo, AcademicRepo: academicRepo,
	})
	if err != nil {
		t.Fatal(err)
	}
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

func mustNewTestRouter(t *testing.T, cfg *config.Config, pinger *mockPinger) http.Handler {
	t.Helper()
	router, err := NewRouter(RouterConfig{
		Config:       cfg,
		DB:           pinger,
		AuthRepo:     auth.NewMockRepository(),
		TenantRepo:   tenants.NewMockRepository(),
		AcademicRepo: academics.NewMockRepository(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestAcademicRoutesRequireActiveMembershipAndCSRF(t *testing.T) {
	authRepo := auth.NewMockRepository()
	password := "a sufficiently long fictional academic test password"
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := &auth.User{
		ID: uuid.New(), Email: "academic-teacher@school.invalid",
		PasswordHash: passwordHash, FullName: "Fictional Teacher", Status: auth.UserStatusActive,
	}
	if err := authRepo.CreateUser(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	academicRepo := academics.NewMockRepository()
	academicRepo.AddMembership("school-a", uuid.New(), user.ID, "TEACHER")
	router, err := NewRouter(RouterConfig{
		Config: &config.Config{
			Environment: "test", PublicWebURL: "http://localhost:3000",
			CORSAllowedOrigins: []string{"http://localhost:3000"},
		},
		DB: &mockPinger{}, AuthRepo: authRepo,
		TenantRepo: tenants.NewMockRepository(), AcademicRepo: academicRepo,
	})
	if err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(authRepo)
	_, sessionToken, csrfToken, err := authService.Login(t.Context(), user.Email, password, "127.0.0.1", "test-agent", "login")
	if err != nil {
		t.Fatal(err)
	}

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/school-a/academic-sessions", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated academic read should be rejected, status=%d", unauthenticated.Code)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/school-a/academic-sessions", nil)
	getRequest.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("active school member should read academic sessions, status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	const body = `{"name":"2026/2027","starts_on":"2026-09-01","ends_on":"2027-07-31"}`
	post := func(includeCSRF bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/school-a/academic-sessions", strings.NewReader(body))
		request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
		request.Header.Set("Origin", "http://localhost:3000")
		request.Header.Set("Content-Type", "application/json")
		if includeCSRF {
			request.Header.Set(auth.HeaderCSRF, csrfToken)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := post(false); response.Code != http.StatusForbidden {
		t.Fatalf("academic mutation without CSRF should be rejected, status=%d body=%s", response.Code, response.Body.String())
	}
	if response := post(true); response.Code != http.StatusForbidden {
		t.Fatalf("teacher must not manage academic sessions, status=%d body=%s", response.Code, response.Body.String())
	}
	roleEscalation := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/school-a/academic-sessions",
		strings.NewReader(`{"name":"2026/2027","starts_on":"2026-09-01","ends_on":"2027-07-31","role":"SCHOOL_OWNER","tenant_id":"`+uuid.NewString()+`"}`))
	roleEscalation.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	roleEscalation.Header.Set("Origin", "http://localhost:3000")
	roleEscalation.Header.Set("Content-Type", "application/json")
	roleEscalation.Header.Set(auth.HeaderCSRF, csrfToken)
	roleEscalationResponse := httptest.NewRecorder()
	router.ServeHTTP(roleEscalationResponse, roleEscalation)
	if roleEscalationResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("client-selected role or tenant fields must be rejected, status=%d", roleEscalationResponse.Code)
	}

	inaccessible := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/school-b/academic-sessions", nil)
	inaccessible.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	inaccessibleResponse := httptest.NewRecorder()
	router.ServeHTTP(inaccessibleResponse, inaccessible)
	if inaccessibleResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant academic lookup should be indistinguishable from missing tenant, status=%d", inaccessibleResponse.Code)
	}

	academicRepo.AddMembership("school-a", academicRepo.TenantIDs["school-a"], user.ID, "SCHOOL_OWNER")
	if response := post(true); response.Code != http.StatusCreated {
		t.Fatalf("school owner with valid CSRF should create academic session, status=%d body=%s", response.Code, response.Body.String())
	}
}

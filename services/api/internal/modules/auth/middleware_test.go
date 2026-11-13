package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
)

func TestAuthMiddleware(t *testing.T) {
	repo := NewMockRepository()
	authSvc := NewService(repo)

	// Seed user & session
	user := &User{
		ID:       uuid.New(),
		Email:    "test@school.ng",
		FullName: "Test User",
		Status:   UserStatusActive,
	}

	_ = repo.CreateUser(context.Background(), user)

	rawToken, _ := GenerateSecureToken(32)
	rawCSRF, _ := GenerateSecureToken(32)
	session := &Session{
		ID:            uuid.New(),
		UserID:        user.ID,
		TokenHash:     HashToken(rawToken),
		CSRFTokenHash: HashToken(rawCSRF),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	_ = repo.CreateSession(context.Background(), session)

	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := GetUser(r.Context())
		if u == nil {
			t.Fatal("expected user in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	stack := middleware.RequestID(Authenticate(authSvc)(RequireAuth(RequireCSRF(authSvc)(protectedHandler))))

	// 1. Unauthenticated request -> 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rrUnauth := httptest.NewRecorder()
	stack.ServeHTTP(rrUnauth, reqUnauth)
	if rrUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", rrUnauth.Code)
	}

	// 2. Authenticated GET request without CSRF -> 200 (GET is safe)
	reqAuthGet := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	reqAuthGet.AddCookie(&http.Cookie{Name: CookieName, Value: rawToken})
	rrAuthGet := httptest.NewRecorder()
	stack.ServeHTTP(rrAuthGet, reqAuthGet)
	if rrAuthGet.Code != http.StatusOK {
		t.Errorf("expected 200 for authenticated GET request, got %d", rrAuthGet.Code)
	}

	// 3. Authenticated POST without CSRF -> 403
	reqAuthPostNoCSRF := httptest.NewRequest(http.MethodPost, "/update", nil)
	reqAuthPostNoCSRF.AddCookie(&http.Cookie{Name: CookieName, Value: rawToken})
	rrAuthPostNoCSRF := httptest.NewRecorder()
	stack.ServeHTTP(rrAuthPostNoCSRF, reqAuthPostNoCSRF)
	if rrAuthPostNoCSRF.Code != http.StatusForbidden {
		t.Errorf("expected 403 for mutating POST without CSRF token, got %d", rrAuthPostNoCSRF.Code)
	}

	// 4. Authenticated POST with valid CSRF -> 200
	reqAuthPostValidCSRF := httptest.NewRequest(http.MethodPost, "/update", nil)
	reqAuthPostValidCSRF.AddCookie(&http.Cookie{Name: CookieName, Value: rawToken})
	reqAuthPostValidCSRF.Header.Set(HeaderCSRF, rawCSRF)
	rrAuthPostValidCSRF := httptest.NewRecorder()
	stack.ServeHTTP(rrAuthPostValidCSRF, reqAuthPostValidCSRF)
	if rrAuthPostValidCSRF.Code != http.StatusOK {
		t.Errorf("expected 200 for mutating POST with valid CSRF token, got %d", rrAuthPostValidCSRF.Code)
	}
}

func TestRequireSameOriginRejectsCrossSiteMutations(t *testing.T) {
	handler := RequireSameOrigin([]string{"https://school.example"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	tests := []struct {
		name       string
		origin     string
		fetchSite  string
		wantStatus int
	}{
		{"trusted origin", "https://school.example", "same-origin", http.StatusNoContent},
		{"untrusted origin", "https://attacker.example", "cross-site", http.StatusForbidden},
		{"cross-site metadata", "", "cross-site", http.StatusForbidden},
		{"origin absent for non-browser client", "", "", http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mutating", nil)
			if test.origin != "" {
				req.Header.Set("Origin", test.origin)
			}
			if test.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", test.fetchSite)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d", test.wantStatus, recorder.Code)
			}
		})
	}
}

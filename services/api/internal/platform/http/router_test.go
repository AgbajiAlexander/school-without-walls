package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/school-without-walls/api/internal/config"
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

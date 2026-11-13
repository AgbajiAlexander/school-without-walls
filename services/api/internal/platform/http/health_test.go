package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/school-without-walls/api/internal/platform/middleware"
)

func TestHealthHandler(t *testing.T) {
	h := NewHealthHandler("test")
	handler := middleware.RequestID(http.HandlerFunc(h.Health))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp SuccessResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %v", resp.Data)
	}

	if data["status"] != "healthy" {
		t.Errorf("expected healthy status, got %v", data["status"])
	}
	if data["service"] != "school-without-walls-api" {
		t.Errorf("expected service name, got %v", data["service"])
	}
	if data["environment"] != "test" {
		t.Errorf("expected test environment, got %v", data["environment"])
	}
}

// mockPinger implements db.Pinger for unit testing
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(ctx context.Context) error {
	return m.err
}

func TestReadyHandlerSuccess(t *testing.T) {
	h := NewReadyHandler(&mockPinger{err: nil})
	handler := middleware.RequestID(http.HandlerFunc(h.Ready))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp SuccessResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok || data["status"] != "ready" || data["database"] != "connected" {
		t.Errorf("unexpected ready payload: %v", resp.Data)
	}
}

func TestReadyHandlerFailure(t *testing.T) {
	h := NewReadyHandler(&mockPinger{err: errors.New("connection refused")})
	handler := middleware.RequestID(http.HandlerFunc(h.Ready))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	if resp.Error.Code != ErrCodeServiceUnavailable {
		t.Errorf("expected error code %s, got %s", ErrCodeServiceUnavailable, resp.Error.Code)
	}
}

func TestReadyHandlerNilPinger(t *testing.T) {
	h := NewReadyHandler(nil)
	handler := middleware.RequestID(http.HandlerFunc(h.Ready))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}
}

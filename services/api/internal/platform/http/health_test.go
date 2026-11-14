package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	const diagnostic = "password=private-pass user=private-user host=private-db.invalid dbname=private-db token=private-token"
	h := NewReadyHandler(&mockPinger{err: errors.New(diagnostic)})
	handler := middleware.RequestID(http.HandlerFunc(h.Ready))
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

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
	if strings.Contains(rr.Body.String(), diagnostic) ||
		strings.Contains(rr.Body.String(), "private-pass") ||
		strings.Contains(rr.Body.String(), "private-user") ||
		strings.Contains(rr.Body.String(), "private-db") ||
		strings.Contains(rr.Body.String(), "private-token") {
		t.Fatal("readiness response exposed database diagnostics")
	}
	if !strings.Contains(rr.Body.String(), "Database is unavailable") ||
		!strings.Contains(rr.Body.String(), "disconnected") {
		t.Fatalf("readiness response should contain only generic status information: %s", rr.Body.String())
	}
	if strings.Contains(logOutput.String(), diagnostic) ||
		strings.Contains(logOutput.String(), "private-pass") ||
		strings.Contains(logOutput.String(), "private-user") ||
		strings.Contains(logOutput.String(), "private-db") ||
		strings.Contains(logOutput.String(), "private-token") {
		t.Fatal("readiness logs exposed database diagnostics")
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
	if strings.Contains(rr.Body.String(), "uninitialized") {
		t.Fatal("readiness response should use generic unavailable wording")
	}
}

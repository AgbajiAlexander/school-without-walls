package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/school-without-walls/api/internal/platform/middleware"
)

func TestJSONSuccessResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-req-id-123")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	data := map[string]string{"name": "School Without Walls", "stage": "PRIMARY"}
	JSON(rr, req, http.StatusOK, data)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("expected application/json; charset=utf-8, got %s", contentType)
	}

	var resp SuccessResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	if resp.Meta.RequestID != "test-req-id-123" {
		t.Errorf("expected request_id test-req-id-123, got %s", resp.Meta.RequestID)
	}

	m, ok := resp.Data.(map[string]any)
	if !ok || m["name"] != "School Without Walls" {
		t.Errorf("unexpected payload: %v", resp.Data)
	}
}

func TestErrorJSONResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "err-req-id-456")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	ErrorJSON(rr, req, http.StatusForbidden, ErrCodeForbidden, "Cross-tenant access denied", map[string]string{"tenant": "unauthorized"})

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON error: %v", err)
	}

	if resp.Error.Code != ErrCodeForbidden {
		t.Errorf("expected code %s, got %s", ErrCodeForbidden, resp.Error.Code)
	}
	if resp.Error.Message != "Cross-tenant access denied" {
		t.Errorf("unexpected message: %s", resp.Error.Message)
	}
	if resp.Error.RequestID != "err-req-id-456" {
		t.Errorf("expected request_id err-req-id-456, got %s", resp.Error.RequestID)
	}
}

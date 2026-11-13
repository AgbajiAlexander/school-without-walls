package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddlewareGeneratesNewID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected non-empty request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resID := rr.Header().Get(HeaderRequestID)
	if resID == "" {
		t.Error("expected X-Request-ID response header")
	}
}

func TestRequestIDMiddlewarePropagatesExistingID(t *testing.T) {
	customID := "custom-uuid-12345"
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID != customID {
			t.Errorf("expected propagated request ID %s, got %s", customID, reqID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderRequestID, customID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resID := rr.Header().Get(HeaderRequestID)
	if resID != customID {
		t.Errorf("expected header %s, got %s", customID, resID)
	}
}

func TestGetRequestIDFallback(t *testing.T) {
	// Nil context
	id1 := GetRequestID(nil)
	if id1 == "" {
		t.Error("expected generated UUID for nil context")
	}

	// Empty context
	id2 := GetRequestID(context.Background())
	if id2 == "" {
		t.Error("expected generated UUID for empty context")
	}
}

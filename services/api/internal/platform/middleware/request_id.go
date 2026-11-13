package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const (
	RequestIDKey    contextKey = "request_id"
	HeaderRequestID            = "X-Request-ID"
)

// RequestID injects or propagates a request ID for tracing.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(HeaderRequestID)
		if reqID == "" {
			// Generate a new UUIDv4 for tracing
			reqID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		w.Header().Set(HeaderRequestID, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request ID from context or generates a fallback UUID.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return uuid.New().String()
	}
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		return reqID
	}
	return uuid.New().String()
}

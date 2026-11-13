package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer recovers from panics, logs the stack trace, and returns a standard JSON error response.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				reqID := GetRequestID(r.Context())
				stack := string(debug.Stack())

				slog.Error("panic_recovered",
					slog.String("request_id", reqID),
					slog.Any("panic", rvr),
					slog.String("stack", stack),
				)

				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)

				errPayload := map[string]any{
					"error": map[string]any{
						"code":       "INTERNAL_ERROR",
						"message":    fmt.Sprintf("Internal server error: %v", rvr),
						"details":    map[string]any{},
						"request_id": reqID,
					},
				}

				_ = json.NewEncoder(w).Encode(errPayload)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

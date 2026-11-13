package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logger returns a middleware that logs incoming requests using slog.
func Logger() func(next http.Handler) http.Handler {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			reqID := GetRequestID(r.Context())
			duration := time.Since(start)

			logger.Info("http_request",
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", redactedPath(r.URL.Path)),
				slog.Int("status", rw.statusCode),
				slog.Duration("latency", duration),
			)
		})
	}
}

func redactedPath(path string) string {
	const invitationPrefix = "/api/v1/invitations/"
	if !strings.HasPrefix(path, invitationPrefix) {
		return path
	}
	rest := strings.TrimPrefix(path, invitationPrefix)
	separator := strings.IndexByte(rest, '/')
	if separator < 0 {
		return invitationPrefix + "[redacted]"
	}
	return invitationPrefix + "[redacted]" + rest[separator:]
}

package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/platform/db"
	"github.com/school-without-walls/api/internal/platform/middleware"
)

// RouterConfig holds dependencies for building the HTTP router.
type RouterConfig struct {
	Config *config.Config
	DB     db.Pinger
}

// NewRouter constructs the Chi HTTP router with standard middleware and routes.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Base middleware stack
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger())
	r.Use(chimw.RealIP)
	r.Use(corsMiddleware(cfg.Config.CORSAllowedOrigins))

	// Custom 404 / 405 matching standard JSON error envelopes
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		ErrorJSON(w, req, http.StatusNotFound, ErrCodeNotFound, "Resource not found", map[string]any{
			"path": req.URL.Path,
		})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		ErrorJSON(w, req, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", map[string]any{
			"method": req.Method,
		})
	})

	healthH := NewHealthHandler(cfg.Config.Environment)
	readyH := NewReadyHandler(cfg.DB)

	// Root operational probes
	r.Get("/health", healthH.Health)
	r.Get("/ready", readyH.Ready)

	// API v1 namespace
	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Get("/health", healthH.Health)
		v1.Get("/ready", readyH.Ready)
	})

	return r
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		allowedMap[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowedMap["*"] || allowedMap[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID, X-CSRF-Token")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "300")
			}

			if strings.EqualFold(r.Method, http.MethodOptions) {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

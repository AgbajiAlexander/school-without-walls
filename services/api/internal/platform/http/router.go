package http

import (
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	"github.com/school-without-walls/api/internal/platform/middleware"
)

// RouterConfig holds dependencies for building the HTTP router.
type RouterConfig struct {
	Config   *config.Config
	DB       db.Pinger
	AuthRepo auth.Repository
}

// NewRouter constructs the Chi HTTP router with standard middleware and routes.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Initialize repository
	authRepo := cfg.AuthRepo
	if authRepo == nil {
		if dbWrapper, ok := cfg.DB.(*db.DB); ok && dbWrapper != nil && dbWrapper.Pool != nil {
			authRepo = auth.NewPostgresRepository(dbWrapper.Pool)
		} else {
			authRepo = auth.NewMockRepository()
		}
	}

	authSvc := auth.NewService(authRepo)
	isSecure := cfg.Config != nil && (cfg.Config.Environment == "production" || cfg.Config.Environment == "staging")
	var authH *auth.Handler
	var allowedOrigins []string
	var trustedProxyNets []*net.IPNet
	var mailer auth.Mailer
	if cfg.Config != nil {
		allowedOrigins = cfg.Config.CORSAllowedOrigins
		trustedProxyNets = cfg.Config.TrustedProxyNets
		if cfg.Config.SMTPHost != "" {
			mailer = auth.NewSMTPMailer(cfg.Config.SMTPHost, cfg.Config.SMTPPort,
				cfg.Config.SMTPUsername, cfg.Config.SMTPPassword, cfg.Config.SMTPFrom, cfg.Config.PublicWebURL)
		}
	}
	authH = auth.NewHandler(authSvc, isSecure, mailer, trustedProxyNets)

	// Base middleware stack
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger())
	if cfg.Config != nil {
		r.Use(auth.RequireSameOrigin(allowedOrigins))
		r.Use(corsMiddleware(allowedOrigins))
	}
	r.Use(auth.Authenticate(authSvc, isSecure))

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

	env := "development"
	if cfg.Config != nil {
		env = cfg.Config.Environment
	}
	healthH := NewHealthHandler(env)
	readyH := NewReadyHandler(cfg.DB)

	// Root operational probes
	r.Get("/health", healthH.Health)
	r.Get("/ready", readyH.Ready)

	// API v1 namespace
	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Get("/health", healthH.Health)
		v1.Get("/ready", readyH.Ready)

		// Public authentication routes
		v1.Route("/auth", func(a chi.Router) {
			a.Post("/login", authH.Login)
			a.Post("/forgot-password", authH.ForgotPassword)
			a.Post("/reset-password", authH.ResetPassword)

			// Protected auth endpoints
			a.Group(func(protected chi.Router) {
				protected.Use(auth.RequireAuth)
				protected.Use(auth.RequireCSRF(authSvc))
				protected.Get("/me", authH.Me)
				protected.Post("/logout", authH.Logout)
				protected.Get("/sessions", authH.ListSessions)
				protected.Delete("/sessions/{id}", authH.RevokeSession)
			})
		})

		// Invitation endpoints
		v1.Route("/invitations", func(inv chi.Router) {
			inv.Get("/{token}/verify", authH.VerifyInvitation)
			inv.Post("/accept", authH.AcceptInvitation)
		})
	})

	return r
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		allowedMap[strings.TrimRight(o, "/")] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowedMap[strings.TrimRight(origin, "/")] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, X-Request-ID, X-CSRF-Token")
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

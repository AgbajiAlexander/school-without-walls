package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type userContextKey string
type sessionContextKey string

const (
	UserKey          userContextKey    = "sww_auth_user"
	SessionKey       sessionContextKey = "sww_auth_session"
	CookieName                         = "sww_session"
	HeaderCSRF                         = "X-CSRF-Token"
	SecureCookieName                   = "__Host-sww_session"
)

// Authenticate extracts session cookie and validates it without rejecting unauthenticated requests.
func Authenticate(authSvc *Service, secureCookie ...bool) func(http.Handler) http.Handler {
	useSecureCookie := len(secureCookie) > 0 && secureCookie[0]
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken := extractSessionToken(r, useSecureCookie)
			if rawToken != "" {
				session, user, err := authSvc.ValidateSession(r.Context(), rawToken)
				if err == nil && user != nil && session != nil {
					ctx := context.WithValue(r.Context(), UserKey, user)
					ctx = context.WithValue(ctx, SessionKey, session)
					r = r.WithContext(ctx)
				} else if err != nil && !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrAccountSuspended) {
					response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "Unable to validate session", nil)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth ensures a valid authenticated session exists in context.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r.Context())
		session := GetSession(r.Context())

		if user == nil || session == nil {
			response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireCSRF verifies CSRF token for mutating requests when authenticated.
func RequireCSRF(authSvc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isSafeMethod(r.Method) {
				session := GetSession(r.Context())
				if session != nil {
					csrfToken := r.Header.Get(HeaderCSRF)
					if csrfToken == "" || !authSvc.VerifyCSRF(session, csrfToken) {
						response.ErrorJSON(w, r, http.StatusForbidden, response.ErrCodeForbidden, "Invalid or missing CSRF token", nil)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireSameOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isSafeMethod(r.Method) {
				if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
					response.ErrorJSON(w, r, http.StatusForbidden, response.ErrCodeForbidden, "Cross-site request denied", nil)
					return
				}
				if origin := strings.TrimRight(r.Header.Get("Origin"), "/"); origin != "" {
					if _, ok := allowed[origin]; !ok {
						response.ErrorJSON(w, r, http.StatusForbidden, response.ErrCodeForbidden, "Request origin is not allowed", nil)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractSessionToken(r *http.Request, secureCookie bool) string {
	name := CookieName
	if secureCookie {
		name = SecureCookieName
	}
	if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// GetUser retrieves the authenticated user from context.
func GetUser(ctx context.Context) *User {
	if ctx == nil {
		return nil
	}
	if u, ok := ctx.Value(UserKey).(*User); ok {
		return u
	}
	return nil
}

// GetSession retrieves the active session from context.
func GetSession(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	if s, ok := ctx.Value(SessionKey).(*Session); ok {
		return s
	}
	return nil
}

// Helper to get request ID from context
func getRequestID(ctx context.Context) string {
	return middleware.GetRequestID(ctx)
}

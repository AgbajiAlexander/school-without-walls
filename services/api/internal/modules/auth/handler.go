package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

const csrfCookieName = "sww_csrf"
const secureCSRFcookieName = "__Host-sww_csrf"

type Handler struct {
	authService      *Service
	mailer           Mailer
	isSecure         bool
	trustedProxyNets []*net.IPNet
}

func NewHandler(authService *Service, isSecure bool, mailer Mailer, trustedProxyNets []*net.IPNet) *Handler {
	return &Handler{
		authService:      authService,
		mailer:           mailer,
		isSecure:         isSecure,
		trustedProxyNets: trustedProxyNets,
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	var req LoginRequest
	if err := decodeRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	ip := extractClientIP(r, h.trustedProxyNets)
	if h.rateLimited(w, r, "login-ip", ip, 20, 5*time.Minute) {
		return
	}
	if h.rateLimited(w, r, "login-account", strings.ToLower(strings.TrimSpace(req.Email)), 5, 15*time.Minute) {
		return
	}

	user, rawSessionToken, rawCSRFToken, err := h.authService.Login(r.Context(), req.Email, req.Password, ip, r.UserAgent(), reqID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrAccountSuspended), errors.Is(err, ErrAccountPendingActivation):
			apphttp.ErrorJSON(w, r, http.StatusUnauthorized, apphttp.ErrCodeUnauthenticated, "Invalid email or password", nil)
		default:
			slog.Error("authentication login failed", "request_id", reqID, "error", err)
			apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Authentication failed", nil)
		}
		return
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	setSessionCookie(w, h.isSecure, rawSessionToken, expiresAt)
	setCSRFCookie(w, h.isSecure, rawCSRFToken, expiresAt)
	apphttp.JSON(w, r, http.StatusOK, LoginResponseData{User: user.ToDTO(), CSRFToken: rawCSRFToken})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	session := GetSession(r.Context())
	user := GetUser(r.Context())
	if session == nil || user == nil {
		apphttp.ErrorJSON(w, r, http.StatusUnauthorized, apphttp.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	if err := h.authService.Logout(r.Context(), session.ID, user.ID, reqID); err != nil {
		slog.Error("session revocation failed", "request_id", reqID, "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to revoke session", nil)
		return
	}
	clearAuthCookies(w, h.isSecure)
	apphttp.JSON(w, r, http.StatusOK, map[string]string{"message": "Successfully logged out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	session := GetSession(r.Context())
	if user == nil || session == nil {
		apphttp.ErrorJSON(w, r, http.StatusUnauthorized, apphttp.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}

	csrfToken := ""
	if cookie, err := r.Cookie(csrfCookieNameFor(h.isSecure)); err == nil && h.authService.VerifyCSRF(session, cookie.Value) {
		csrfToken = cookie.Value
	}
	if csrfToken == "" {
		var err error
		csrfToken, err = GenerateSecureToken(32)
		if err != nil {
			slog.Error("CSRF token generation failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
			apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to establish request security", nil)
			return
		}
		if err := h.authService.UpdateCSRFToken(r.Context(), session.ID, csrfToken); err != nil {
			slog.Error("CSRF token update failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
			apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to establish request security", nil)
			return
		}
		setCSRFCookie(w, h.isSecure, csrfToken, session.ExpiresAt)
	}
	apphttp.JSON(w, r, http.StatusOK, CurrentUserResponseData{
		User: user.ToDTO(), Session: session.ToDTO(), CSRFToken: csrfToken,
	})
}

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	sessions, err := h.authService.ActiveSessions(r.Context(), user.ID)
	if err != nil {
		slog.Error("session listing failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to list sessions", nil)
		return
	}
	items := make([]SessionDTO, 0, len(sessions))
	for i := range sessions {
		items = append(items, sessions[i].ToDTO())
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"sessions": items})
}

func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Invalid session ID", nil)
		return
	}
	if err := h.authService.RevokeUserSession(r.Context(), user.ID, sessionID, middleware.GetRequestID(r.Context())); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			apphttp.ErrorJSON(w, r, http.StatusNotFound, apphttp.ErrCodeNotFound, "Session not found", nil)
			return
		}
		slog.Error("session revocation failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to revoke session", nil)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]string{"message": "Session revoked"})
}

func (h *Handler) VerifyInvitation(w http.ResponseWriter, r *http.Request) {
	inv, err := h.authService.VerifyInvitation(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Invitation token is invalid or expired", nil)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{
		"invited_email": inv.InvitedEmail,
		"expires_at":    inv.ExpiresAt,
	})
}

func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	var req AcceptInvitationRequest
	if err := decodeRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	ip := extractClientIP(r, h.trustedProxyNets)
	if h.rateLimited(w, r, "invitation-ip", ip, 20, 15*time.Minute) {
		return
	}
	if h.rateLimited(w, r, "invitation-token", HashToken(req.Token), 5, 15*time.Minute) {
		return
	}

	user, rawSessionToken, rawCSRFToken, err := h.authService.AcceptInvitation(
		r.Context(), req.Token, req.FullName, req.Password, ip, r.UserAgent(), reqID,
	)
	if err != nil {
		if errors.Is(err, ErrInvitationIdentityExists) {
			apphttp.ErrorJSON(w, r, http.StatusConflict, apphttp.ErrCodeConflict, "Sign in to the existing account before accepting this invitation", nil)
			return
		}
		if errors.Is(err, ErrInvitationNotFound) {
			apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Invitation token is invalid, expired, or already used", nil)
			return
		}
		if isPasswordPolicyError(err) {
			apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, err.Error(), nil)
			return
		}
		slog.Error("invitation acceptance failed", "request_id", reqID, "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to accept invitation", nil)
		return
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	setSessionCookie(w, h.isSecure, rawSessionToken, expiresAt)
	setCSRFCookie(w, h.isSecure, rawCSRFToken, expiresAt)
	apphttp.JSON(w, r, http.StatusCreated, LoginResponseData{User: user.ToDTO(), CSRFToken: rawCSRFToken})
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	if h.mailer == nil {
		apphttp.ErrorJSON(w, r, http.StatusServiceUnavailable, apphttp.ErrCodeServiceUnavailable, "Password recovery is temporarily unavailable", nil)
		return
	}
	var req ForgotPasswordRequest
	if err := decodeRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	ip := extractClientIP(r, h.trustedProxyNets)
	account := strings.ToLower(strings.TrimSpace(req.Email))
	if parsed, err := mail.ParseAddress(account); err == nil {
		account = strings.ToLower(parsed.Address)
	}
	if h.rateLimited(w, r, "password-reset-ip", ip, 10, time.Hour) {
		return
	}
	if h.rateLimited(w, r, "password-reset-account", account, 3, time.Hour) {
		return
	}

	token, err := h.authService.RequestPasswordReset(r.Context(), account, reqID)
	if err == nil && token != "" {
		user, lookupErr := h.authService.repo.GetUserByEmail(r.Context(), account)
		if lookupErr == nil {
			if sendErr := h.mailer.SendPasswordReset(r.Context(), user.Email, token, time.Now().Add(15*time.Minute)); sendErr != nil {
				slog.Error("password reset email delivery failed", "request_id", reqID, "error", sendErr)
			}
		} else {
			slog.Error("password reset recipient lookup failed", "request_id", reqID, "error", lookupErr)
		}
	} else if err != nil {
		slog.Error("password reset request failed", "request_id", reqID, "error", err)
	}
	apphttp.JSON(w, r, http.StatusAccepted, map[string]string{
		"message": "If an account matches that email address, reset instructions will be sent when email delivery is available.",
	})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	var req ResetPasswordRequest
	if err := decodeRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	ip := extractClientIP(r, h.trustedProxyNets)
	if h.rateLimited(w, r, "password-reset-confirm", ip, 5, 15*time.Minute) {
		return
	}
	if err := h.authService.ResetPassword(r.Context(), req.Token, req.NewPassword, reqID); err != nil {
		if errors.Is(err, ErrPasswordResetNotFound) {
			apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Password reset token is invalid, expired, or already used", nil)
			return
		}
		if isPasswordPolicyError(err) {
			apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, err.Error(), nil)
			return
		}
		slog.Error("password reset confirmation failed", "request_id", reqID, "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to reset password", nil)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]string{"message": "Password successfully reset. Please sign in with your new credentials."})
}

func isPasswordPolicyError(err error) bool {
	return errors.Is(err, ErrPasswordTooShort) ||
		errors.Is(err, ErrPasswordTooLong) ||
		errors.Is(err, ErrPasswordCommon) ||
		errors.Is(err, ErrPasswordInvalidUTF8)
}

func (h *Handler) rateLimited(w http.ResponseWriter, r *http.Request, action, key string, attempts int, window time.Duration) bool {
	retryAfter, err := h.authService.AllowRateLimit(r.Context(), action, key, attempts, window)
	if err != nil {
		slog.Error("authentication rate-limit check failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to process authentication request", nil)
		return true
	}
	if retryAfter <= 0 {
		return false
	}
	seconds := int(retryAfter.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	apphttp.ErrorJSON(w, r, http.StatusTooManyRequests, apphttp.ErrCodeRateLimited, "Too many attempts. Please try again later.", map[string]any{"retry_after_seconds": seconds})
	return true
}

func decodeRequest(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func cookieName(secure bool) string {
	if secure {
		return SecureCookieName
	}
	return CookieName
}

func csrfCookieNameFor(secure bool) string {
	if secure {
		return secureCSRFcookieName
	}
	return csrfCookieName
}

func setSessionCookie(w http.ResponseWriter, secure bool, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(secure), Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expiresAt, MaxAge: maxAge,
	})
}

func setCSRFCookie(w http.ResponseWriter, secure bool, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieNameFor(secure), Value: token, Path: "/", HttpOnly: false,
		Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expiresAt,
		MaxAge: int(time.Until(expiresAt).Seconds()),
	})
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{cookieName(secure), csrfCookieNameFor(secure)} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", HttpOnly: name == cookieName(secure),
			Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
	}
}

func extractClientIP(r *http.Request, trustedProxyNets []*net.IPNet) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remoteIP := net.ParseIP(strings.Trim(remoteHost, "[]"))
	if remoteIP == nil {
		return "unknown"
	}
	isTrusted := func(ip net.IP) bool {
		for _, network := range trustedProxyNets {
			if network.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(remoteIP) {
		return remoteIP.String()
	}

	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(chain) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(chain[i]))
		if ip == nil {
			return remoteIP.String()
		}
		if !isTrusted(ip) {
			return ip.String()
		}
	}
	return remoteIP.String()
}

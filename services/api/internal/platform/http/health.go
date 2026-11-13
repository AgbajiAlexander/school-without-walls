package http

import (
	"context"
	"net/http"
	"time"

	"github.com/school-without-walls/api/internal/platform/db"
)

var startTime = time.Now()

type HealthHandler struct {
	env string
}

func NewHealthHandler(env string) *HealthHandler {
	return &HealthHandler{env: env}
}

// Health (Liveness) returns 200 OK with runtime information.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).Truncate(time.Second).Seconds()

	data := map[string]any{
		"status":         "healthy",
		"service":        "school-without-walls-api",
		"version":        "0.1.0",
		"environment":    h.env,
		"uptime_seconds": uptime,
	}

	JSON(w, r, http.StatusOK, data)
}

type ReadyHandler struct {
	pinger db.Pinger
}

func NewReadyHandler(pinger db.Pinger) *ReadyHandler {
	return &ReadyHandler{pinger: pinger}
}

// Ready (Readiness) checks downstream dependencies (e.g., PostgreSQL).
func (h *ReadyHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if h.pinger == nil {
		ErrorJSON(w, r, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Database pinger uninitialized", map[string]any{
			"database": "uninitialized",
		})
		return
	}

	if err := h.pinger.Ping(ctx); err != nil {
		ErrorJSON(w, r, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Database not ready: "+err.Error(), map[string]any{
			"database": "disconnected",
		})
		return
	}

	data := map[string]any{
		"status":   "ready",
		"database": "connected",
	}

	JSON(w, r, http.StatusOK, data)
}

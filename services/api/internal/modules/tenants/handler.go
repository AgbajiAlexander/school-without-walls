package tenants

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

type Handler struct {
	service       *Service
	captureMailer *CaptureMailer
}

func NewHandler(service *Service, captureMailer *CaptureMailer) *Handler {
	return &Handler{service: service, captureMailer: captureMailer}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	tenants, err := h.service.List(r.Context(), actor.ID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"tenants": tenants})
}

func (h *Handler) Provision(w http.ResponseWriter, r *http.Request) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	var request ProvisionRequest
	if err := decodeTenantRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	tenant, err := h.service.Provision(r.Context(), actor.ID, request, middleware.GetRequestID(r.Context()))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, tenant)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	tenant, err := h.service.GetSettings(r.Context(), actor.ID, chi.URLParam(r, "slug"))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, tenant)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	var request SettingsRequest
	if err := decodeTenantRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	tenant, err := h.service.UpdateSettings(r.Context(), actor.ID, chi.URLParam(r, "slug"), &request, middleware.GetRequestID(r.Context()))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, tenant)
}

func (h *Handler) CapturedMail(w http.ResponseWriter, r *http.Request) {
	if h.captureMailer == nil {
		apphttp.ErrorJSON(w, r, http.StatusNotFound, apphttp.ErrCodeNotFound, "Resource not found", nil)
		return
	}
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	allowed, err := h.service.IsPlatformAdmin(r.Context(), actor.ID)
	if err != nil {
		slog.Error("platform administrator check failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to load captured messages", nil)
		return
	}
	if !allowed {
		apphttp.ErrorJSON(w, r, http.StatusForbidden, apphttp.ErrCodeForbidden, "Platform administrator access required", nil)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"messages": h.captureMailer.Messages()})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Invalid tenant data", nil)
	case errors.Is(err, ErrForbidden):
		apphttp.ErrorJSON(w, r, http.StatusForbidden, apphttp.ErrCodeForbidden, "Access denied", nil)
	case errors.Is(err, ErrNotFound):
		apphttp.ErrorJSON(w, r, http.StatusNotFound, apphttp.ErrCodeNotFound, "Tenant not found", nil)
	case errors.Is(err, ErrConflict):
		apphttp.ErrorJSON(w, r, http.StatusConflict, "TENANT_SLUG_CONFLICT", "That school address is already in use", nil)
	default:
		slog.Error("tenant operation failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Tenant operation failed", nil)
	}
}

func tenantActor(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	actor := auth.GetUser(r.Context())
	if actor == nil || actor.ID == uuid.Nil {
		apphttp.ErrorJSON(w, r, http.StatusUnauthorized, apphttp.ErrCodeUnauthenticated, "Authentication required", nil)
		return nil, false
	}
	return actor, true
}

func decodeTenantRequest(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}

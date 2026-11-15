package academics

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
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	sessions, err := h.service.ListSessions(r.Context(), actor.ID, chi.URLParam(r, "slug"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"academic_sessions": sessions})
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var request CreateSessionRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	session, err := h.service.CreateSession(r.Context(), actor.ID, chi.URLParam(r, "slug"), request, middleware.GetRequestID(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, session)
}

func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	actor, sessionID, ok := academicRequestActor(w, r)
	if !ok {
		return
	}
	session, err := h.service.GetSession(r.Context(), actor.ID, chi.URLParam(r, "slug"), sessionID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, session)
}

func (h *Handler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	actor, sessionID, ok := academicRequestActor(w, r)
	if !ok {
		return
	}
	var request UpdateSessionRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	session, err := h.service.UpdateSession(r.Context(), actor.ID, chi.URLParam(r, "slug"), sessionID, request, middleware.GetRequestID(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, session)
}

func (h *Handler) GetTerms(w http.ResponseWriter, r *http.Request) {
	actor, sessionID, ok := academicRequestActor(w, r)
	if !ok {
		return
	}
	terms, err := h.service.GetTerms(r.Context(), actor.ID, chi.URLParam(r, "slug"), sessionID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"terms": terms})
}

func (h *Handler) ConfigureTerms(w http.ResponseWriter, r *http.Request) {
	actor, sessionID, ok := academicRequestActor(w, r)
	if !ok {
		return
	}
	var request ConfigureTermsRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	terms, err := h.service.ConfigureTerms(r.Context(), actor.ID, chi.URLParam(r, "slug"), sessionID, request, middleware.GetRequestID(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"terms": terms})
}

func (h *Handler) ActivateSession(w http.ResponseWriter, r *http.Request) {
	h.changeStatus(w, r, true)
}

func (h *Handler) ArchiveSession(w http.ResponseWriter, r *http.Request) {
	h.changeStatus(w, r, false)
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request, activate bool) {
	actor, sessionID, ok := academicRequestActor(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	requestID := middleware.GetRequestID(r.Context())
	var session *Session
	var err error
	if activate {
		session, err = h.service.ActivateSession(r.Context(), actor.ID, slug, sessionID, requestID)
	} else {
		session, err = h.service.ArchiveSession(r.Context(), actor.ID, slug, sessionID, requestID)
	}
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, session)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Invalid academic data", nil)
	case errors.Is(err, ErrForbidden):
		apphttp.ErrorJSON(w, r, http.StatusForbidden, apphttp.ErrCodeForbidden, "Access denied", nil)
	case errors.Is(err, ErrNotFound):
		apphttp.ErrorJSON(w, r, http.StatusNotFound, apphttp.ErrCodeNotFound, "Resource not found", nil)
	case errors.Is(err, ErrConflict):
		apphttp.ErrorJSON(w, r, http.StatusConflict, apphttp.ErrCodeConflict, "Academic change conflicts with the current school structure", nil)
	default:
		slog.Error("academic operation failed", "request_id", middleware.GetRequestID(r.Context()), "error", err)
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Academic operation failed", nil)
	}
}

func academicActor(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	actor := auth.GetUser(r.Context())
	if actor == nil || actor.ID == uuid.Nil {
		apphttp.ErrorJSON(w, r, http.StatusUnauthorized, apphttp.ErrCodeUnauthenticated, "Authentication required", nil)
		return nil, false
	}
	return actor, true
}

func academicRequestActor(w http.ResponseWriter, r *http.Request) (*auth.User, uuid.UUID, bool) {
	actor, ok := academicActor(w, r)
	if !ok {
		return nil, uuid.Nil, false
	}
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		apphttp.ErrorJSON(w, r, http.StatusNotFound, apphttp.ErrCodeNotFound, "Resource not found", nil)
		return nil, uuid.Nil, false
	}
	return actor, sessionID, true
}

func decodeAcademicRequest(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
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

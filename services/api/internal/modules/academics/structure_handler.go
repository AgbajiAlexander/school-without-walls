package academics

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

// StructureHandler serves SWW-004B academic structure endpoints.
// Authentication is enforced by the router middleware and academicActor;
// tenant membership and write permissions are checked in the repository.
type StructureHandler struct {
	service *StructureService
}

func NewStructureHandler(service *StructureService) *StructureHandler {
	return &StructureHandler{service: service}
}

func (h *StructureHandler) ListGradeLevels(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	levels, err := h.service.ListGradeLevels(r.Context(), actor.ID, chi.URLParam(r, "slug"))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"grade_levels": levels})
}

func (h *StructureHandler) CreateGradeLevel(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var request CreateGradeLevelRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	level, err := h.service.CreateGradeLevel(r.Context(), actor.ID, chi.URLParam(r, "slug"), request, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, level)
}

func (h *StructureHandler) ListClasses(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	sessionID, err := uuid.Parse(r.URL.Query().Get("session_id"))
	if err != nil || sessionID == uuid.Nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Valid session_id query parameter required", nil)
		return
	}
	classes, err := h.service.ListClasses(r.Context(), actor.ID, chi.URLParam(r, "slug"), sessionID)
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"classes": classes})
}

func (h *StructureHandler) CreateClass(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var request CreateClassRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	class, err := h.service.CreateClass(r.Context(), actor.ID, chi.URLParam(r, "slug"), request, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, class)
}

func (h *StructureHandler) ListClassArms(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	classID, err := uuid.Parse(r.URL.Query().Get("class_id"))
	if err != nil || classID == uuid.Nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Valid class_id query parameter required", nil)
		return
	}
	arms, err := h.service.ListClassArms(r.Context(), actor.ID, chi.URLParam(r, "slug"), classID)
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, map[string]any{"class_arms": arms})
}

func (h *StructureHandler) CreateClassArm(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var request CreateClassArmRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	arm, err := h.service.CreateClassArm(r.Context(), actor.ID, chi.URLParam(r, "slug"), request, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, arm)
}

package academics

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

// TeacherAssignmentHandler exposes school-scoped teacher assignment management.
// Authentication and CSRF are applied by the router. The repository enforces
// tenant isolation and administrator/principal write authorization.
type TeacherAssignmentHandler struct{ service *TeacherAssignmentService }

func NewTeacherAssignmentHandler(service *TeacherAssignmentService) *TeacherAssignmentHandler {
	return &TeacherAssignmentHandler{service: service}
}

func (h *TeacherAssignmentHandler) CreateTeacherAssignment(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var req CreateTeacherAssignmentRequest
	if err := decodeAcademicRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	value, err := h.service.CreateTeacherAssignment(r.Context(), actor.ID, chi.URLParam(r, "slug"), req, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, value)
}

func (h *TeacherAssignmentHandler) ListTeacherAssignments(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	values, err := h.service.ListTeacherAssignments(r.Context(), actor.ID, chi.URLParam(r, "slug"))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, values)
}

func (h *TeacherAssignmentHandler) DeactivateTeacherAssignment(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "assignmentID")
	if !ok {
		return
	}
	value, err := h.service.DeactivateTeacherAssignment(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, value)
}

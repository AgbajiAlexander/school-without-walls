package academics

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

// SubjectHandler exposes the school-scoped subject catalog.
// Authentication and CSRF are applied by the router; the repository rechecks
// school membership and administrative authorization on each operation.
type SubjectHandler struct{ service *SubjectService }

func NewSubjectHandler(service *SubjectService) *SubjectHandler {
	return &SubjectHandler{service: service}
}

func (h *SubjectHandler) CreateSubject(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var req CreateSubjectRequest
	if err := decodeAcademicRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	value, err := h.service.CreateSubject(r.Context(), actor.ID, chi.URLParam(r, "slug"), req, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, value)
}

func (h *SubjectHandler) ListSubjects(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	values, err := h.service.ListSubjects(r.Context(), actor.ID, chi.URLParam(r, "slug"))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, values)
}

func (h *SubjectHandler) UpdateSubject(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "subjectID")
	if !ok {
		return
	}
	var req UpdateSubjectRequest
	if err := decodeAcademicRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	value, err := h.service.UpdateSubject(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, req, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, value)
}

func (h *SubjectHandler) ArchiveSubject(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "subjectID")
	if !ok {
		return
	}
	value, err := h.service.ArchiveSubject(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, value)
}

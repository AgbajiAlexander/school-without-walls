package academics

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

// structureMutationID validates the identifier before invoking the service.
// Authorization and CSRF protections are enforced by the registered router
// middleware, and tenant authorization is enforced in the repository.
func structureMutationID(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil || id == uuid.Nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity,
			apphttp.ErrCodeInvalidInput, "Invalid resource identifier", nil)
		return uuid.Nil, false
	}
	return id, true
}

func (h *StructureHandler) UpdateGradeLevel(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "gradeLevelID")
	if !ok {
		return
	}
	var request UpdateGradeLevelRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity,
			apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	result, err := h.service.UpdateGradeLevel(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, request, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, result)
}

func (h *StructureHandler) ArchiveGradeLevel(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "gradeLevelID")
	if !ok {
		return
	}
	result, err := h.service.ArchiveGradeLevel(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, result)
}

func (h *StructureHandler) UpdateClass(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "classID")
	if !ok {
		return
	}
	var request UpdateClassRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity,
			apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	result, err := h.service.UpdateClass(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, request, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, result)
}

func (h *StructureHandler) ArchiveClass(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "classID")
	if !ok {
		return
	}
	result, err := h.service.ArchiveClass(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, result)
}

func (h *StructureHandler) UpdateClassArm(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "armID")
	if !ok {
		return
	}
	var request UpdateClassArmRequest
	if err := decodeAcademicRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity,
			apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	result, err := h.service.UpdateClassArm(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, request, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, result)
}

func (h *StructureHandler) ArchiveClassArm(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "armID")
	if !ok {
		return
	}
	result, err := h.service.ArchiveClassArm(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, result)
}

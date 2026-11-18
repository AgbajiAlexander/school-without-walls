package academics

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

// SubjectOfferingHandler exposes school-scoped subject offering operations.
// The router enforces authentication and CSRF. The repository enforces tenant
// membership and privileged roles for mutations.
type SubjectOfferingHandler struct{ service *SubjectOfferingService }

func NewSubjectOfferingHandler(service *SubjectOfferingService) *SubjectOfferingHandler {
	return &SubjectOfferingHandler{service: service}
}

func (h *SubjectOfferingHandler) CreateSubjectOffering(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	var req CreateSubjectOfferingRequest
	if err := decodeAcademicRequest(w, r, &req); err != nil {
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Malformed request body", nil)
		return
	}
	value, err := h.service.CreateSubjectOffering(r.Context(), actor.ID, chi.URLParam(r, "slug"), req, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, value)
}

func (h *SubjectOfferingHandler) ListSubjectOfferings(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	values, err := h.service.ListSubjectOfferings(r.Context(), actor.ID, chi.URLParam(r, "slug"))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, values)
}

func (h *SubjectOfferingHandler) ArchiveSubjectOffering(w http.ResponseWriter, r *http.Request) {
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	id, ok := structureMutationID(w, r, "offeringID")
	if !ok {
		return
	}
	value, err := h.service.ArchiveSubjectOffering(r.Context(), actor.ID, chi.URLParam(r, "slug"), id, middleware.GetRequestID(r.Context()))
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusOK, value)
}

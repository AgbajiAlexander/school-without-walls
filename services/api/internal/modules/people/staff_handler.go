package people

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

func (h *Handler) ListStaff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	after, ok := staffCursor(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListStaff(r.Context(), chi.URLParam(r, "slug"), actor, after)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	var input struct {
		StaffInput
		MembershipID uuid.UUID `json:"membership_id"`
	}
	if !decodePeopleRequest(w, r, &input) || !input.normalize() || input.MembershipID == uuid.Nil {
		invalidStaff(w, r)
		return
	}
	record, err := h.repo.CreateStaff(r.Context(), chi.URLParam(r, "slug"), actor, input.MembershipID, input.StaffInput, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, record)
}

func (h *Handler) UpdateStaff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "staffID"))
	if err != nil || id == uuid.Nil {
		writeError(w, r, ErrNotFound)
		return
	}
	var input StaffInput
	if !decodePeopleRequest(w, r, &input) || !input.normalize() {
		invalidStaff(w, r)
		return
	}
	record, err := h.repo.UpdateStaff(r.Context(), chi.URLParam(r, "slug"), actor, id, input, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, record)
}

func invalidStaff(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid staff profile", nil)
}

func (h *Handler) ListStaffCandidates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	after, ok := staffCursor(w, r)
	if !ok {
		return
	}
	page, err := h.repo.ListStaffCandidates(r.Context(), chi.URLParam(r, "slug"), actor, after)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func staffCursor(w http.ResponseWriter, r *http.Request) (*uuid.UUID, bool) {
	var after *uuid.UUID
	if raw := r.URL.Query().Get("after"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid staff cursor", nil)
			return nil, false
		}
		after = &id
	}
	return after, true
}

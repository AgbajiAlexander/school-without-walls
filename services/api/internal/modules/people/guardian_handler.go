package people

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

func (h *Handler) ListGuardians(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	var student, after *uuid.UUID
	for _, query := range []struct {
		name string
		dest **uuid.UUID
	}{{"student_id", &student}, {"after", &after}} {
		if raw := r.URL.Query().Get(query.name); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil || id == uuid.Nil {
				response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid guardian list filter", nil)
				return
			}
			*query.dest = &id
		}
	}
	page, err := h.repo.ListGuardians(r.Context(), chi.URLParam(r, "slug"), actor, student, after)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) CreateGuardian(w http.ResponseWriter, r *http.Request) {
	h.saveGuardian(w, r, true)
}

func (h *Handler) UpdateGuardian(w http.ResponseWriter, r *http.Request) {
	h.saveGuardian(w, r, false)
}

func (h *Handler) saveGuardian(w http.ResponseWriter, r *http.Request, create bool) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	var input GuardianInput
	if !decodePeopleRequest(w, r, &input) || !input.normalize() {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid guardian profile", nil)
		return
	}
	var record GuardianRecord
	var err error
	slug, requestID := chi.URLParam(r, "slug"), middleware.GetRequestID(r.Context())
	status := http.StatusOK
	if create {
		record, err = h.repo.CreateGuardian(r.Context(), slug, actor, input, requestID)
		status = http.StatusCreated
	} else {
		id, parseErr := uuid.Parse(chi.URLParam(r, "guardianID"))
		if parseErr != nil || id == uuid.Nil {
			writeError(w, r, ErrNotFound)
			return
		}
		record, err = h.repo.UpdateGuardian(r.Context(), slug, actor, id, input, requestID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, status, record)
}

func (h *Handler) LinkGuardian(w http.ResponseWriter, r *http.Request) {
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	guardian, err := uuid.Parse(chi.URLParam(r, "guardianID"))
	if err != nil || guardian == uuid.Nil {
		writeError(w, r, ErrNotFound)
		return
	}
	var input struct {
		Relationship string `json:"relationship"`
	}
	if !decodePeopleRequest(w, r, &input) || !validProfileText(strings.TrimSpace(input.Relationship), 40) {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid guardian relationship", nil)
		return
	}
	err = h.repo.LinkGuardian(r.Context(), chi.URLParam(r, "slug"), actor, student, guardian, strings.TrimSpace(input.Relationship), middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

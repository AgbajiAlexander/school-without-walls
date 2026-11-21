package people

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

func (h *Handler) ListStudents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	var after *uuid.UUID
	if raw := r.URL.Query().Get("after"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid student cursor", nil)
			return
		}
		after = &id
	}
	page, err := h.repo.ListStudents(r.Context(), chi.URLParam(r, "slug"), actor, after)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) CreateStudent(w http.ResponseWriter, r *http.Request) {
	actor, ok := requestActor(w, r)
	if !ok {
		return
	}
	var input StudentInput
	if !decodePeopleRequest(w, r, &input) || !input.normalize() {
		writeInvalidStudent(w, r)
		return
	}
	result, err := h.repo.CreateStudent(r.Context(), chi.URLParam(r, "slug"), actor, input, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusCreated, result)
}

func (h *Handler) UpdateStudent(w http.ResponseWriter, r *http.Request) {
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	var input StudentInput
	if !decodePeopleRequest(w, r, &input) || !input.normalize() {
		writeInvalidStudent(w, r)
		return
	}
	result, err := h.repo.UpdateStudent(r.Context(), chi.URLParam(r, "slug"), actor, student, input, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, result)
}

func writeInvalidStudent(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid student profile", nil)
}

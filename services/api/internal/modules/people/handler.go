package people

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
	"github.com/school-without-walls/api/internal/platform/response"
)

type Handler struct{ repo *Repository }

func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) GetStudent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	result, err := h.repo.Student(r.Context(), chi.URLParam(r, "slug"), actor, student)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) UnlinkGuardian(w http.ResponseWriter, r *http.Request) {
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	guardian, err := uuid.Parse(chi.URLParam(r, "guardianID"))
	if err != nil {
		writeError(w, r, ErrNotFound)
		return
	}
	err = h.repo.UnlinkGuardian(r.Context(), chi.URLParam(r, "slug"), actor, student, guardian, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TransferEnrollment(w http.ResponseWriter, r *http.Request) {
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	expected, err := uuid.Parse(chi.URLParam(r, "enrollmentID"))
	if err != nil || expected == uuid.Nil {
		writeError(w, r, ErrNotFound)
		return
	}

	var destination TransferDestination
	if !decodePeopleRequest(w, r, &destination) || !destination.valid() {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid enrollment destination", nil)
		return
	}
	result, err := h.repo.TransferEnrollment(r.Context(), chi.URLParam(r, "slug"), actor, student, expected, destination, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, result)
}

func (h *Handler) CreateEnrollment(w http.ResponseWriter, r *http.Request) {
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	var input InitialEnrollment
	if !decodePeopleRequest(w, r, &input) || input.TermID == uuid.Nil || !input.TransferDestination.valid() {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid enrollment placement", nil)
		return
	}
	result, err := h.repo.CreateEnrollment(r.Context(), chi.URLParam(r, "slug"), actor, student, input, middleware.GetRequestID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, result)
}

func (h *Handler) ListEnrollments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, student, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	var before *uuid.UUID
	if raw := r.URL.Query().Get("before"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid enrollment cursor", nil)
			return
		}
		before = &id
	}
	result, err := h.repo.EnrollmentHistory(r.Context(), chi.URLParam(r, "slug"), actor, student, before)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func decodePeopleRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(new(any)) == io.EOF
}

func (destination TransferDestination) valid() bool {
	return destination.ClassID != uuid.Nil && (destination.ClassArmID == nil || *destination.ClassArmID != uuid.Nil)
}

func requestIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	actor, ok := requestActor(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	student, err := uuid.Parse(chi.URLParam(r, "studentID"))
	if err != nil {
		writeError(w, r, ErrNotFound)
		return uuid.Nil, uuid.Nil, false
	}
	return actor, student, true
}

func requestActor(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	user := auth.GetUser(r.Context())
	if user == nil || user.ID == uuid.Nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return uuid.Nil, false
	}
	return user.ID, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		response.ErrorJSON(w, r, http.StatusNotFound, response.ErrCodeNotFound, "Resource not found", nil)
		return
	}
	if errors.Is(err, ErrEnrollmentConflict) {
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "An active enrollment already exists or the placement could not be changed", nil)
		return
	}
	if errors.Is(err, ErrAdmissionNumberConflict) {
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "Admission number is already in use", nil)
		return
	}
	if errors.Is(err, ErrStaffProfileConflict) {
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "Staff code or identity already has a profile", nil)
		return
	}
	slog.Error("people operation failed", "request_id", middleware.GetRequestID(r.Context()))
	response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "People operation failed", nil)
}

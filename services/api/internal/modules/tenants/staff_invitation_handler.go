package tenants

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

type StaffInvitationHandler struct {
	service *StaffInvitationService
}

func NewStaffInvitationHandler(
	service *StaffInvitationService,
) *StaffInvitationHandler {
	return &StaffInvitationHandler{service: service}
}

func (h *StaffInvitationHandler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}

	var request StaffInvitationRequest
	if err := decodeTenantRequest(w, r, &request); err != nil {
		apphttp.ErrorJSON(
			w, r,
			http.StatusUnprocessableEntity,
			apphttp.ErrCodeInvalidInput,
			"Malformed staff invitation request",
			nil,
		)
		return
	}

	result, err := h.service.InviteStaff(
		r.Context(),
		actor.ID,
		chi.URLParam(r, "slug"),
		request,
		middleware.GetRequestID(r.Context()),
	)

	if err != nil {
		var limited *staffInvitationRateLimitError
		switch {
		case errors.As(err, &limited):
			seconds := int((limited.retryAfter + time.Second - 1) / time.Second)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			apphttp.ErrorJSON(w, r, http.StatusTooManyRequests, apphttp.ErrCodeRateLimited,
				"Staff invitation limit reached. Try again later.", map[string]any{"retry_after_seconds": seconds})
		case errors.Is(err, ErrInvalidInput):
			apphttp.ErrorJSON(
				w, r, http.StatusUnprocessableEntity,
				apphttp.ErrCodeInvalidInput,
				"Invalid staff invitation details", nil,
			)
		case errors.Is(err, ErrForbidden):
			apphttp.ErrorJSON(
				w, r, http.StatusForbidden,
				apphttp.ErrCodeForbidden,
				"Access denied", nil,
			)
		case errors.Is(err, ErrConflict):
			apphttp.ErrorJSON(
				w, r, http.StatusConflict,
				"STAFF_INVITATION_CONFLICT",
				"Staff invitation conflicts with an existing invitation or membership",
				nil,
			)
		default:
			slog.Error(
				"staff invitation failed",
				"request_id",
				middleware.GetRequestID(r.Context()),
				"error", err,
			)
			apphttp.ErrorJSON(
				w, r, http.StatusInternalServerError,
				apphttp.ErrCodeInternalError,
				"Unable to create staff invitation", nil,
			)
		}
		return
	}

	apphttp.JSON(w, r, http.StatusCreated, result)
}

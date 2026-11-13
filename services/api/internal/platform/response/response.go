package response

import (
	"encoding/json"
	"net/http"

	"github.com/school-without-walls/api/internal/platform/middleware"
)

type SuccessResponse struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

type Meta struct {
	RequestID string `json:"request_id"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details"`
	RequestID string `json:"request_id"`
}

const (
	ErrCodeUnauthenticated    = "UNAUTHENTICATED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeConflict           = "CONFLICT"
	ErrCodeInvalidInput       = "INVALID_INPUT"
	ErrCodeRateLimited        = "RATE_LIMITED"
	ErrCodeAIUnavailable      = "AI_UNAVAILABLE"
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// JSON writes a JSON success response with status code and sets Content-Type header.
func JSON(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	reqID := middleware.GetRequestID(r.Context())

	resp := SuccessResponse{
		Data: data,
		Meta: Meta{
			RequestID: reqID,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// ErrorJSON writes a standardized JSON error response.
func ErrorJSON(w http.ResponseWriter, r *http.Request, statusCode int, code, message string, details any) {
	reqID := middleware.GetRequestID(r.Context())

	if details == nil {
		details = map[string]any{}
	}

	resp := ErrorResponse{
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: reqID,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

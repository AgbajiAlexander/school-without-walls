package http

import (
	"net/http"

	"github.com/school-without-walls/api/internal/platform/response"
)

type SuccessResponse = response.SuccessResponse
type ErrorResponse = response.ErrorResponse
type Meta = response.Meta
type ErrorBody = response.ErrorBody

const (
	ErrCodeUnauthenticated    = response.ErrCodeUnauthenticated
	ErrCodeForbidden          = response.ErrCodeForbidden
	ErrCodeNotFound           = response.ErrCodeNotFound
	ErrCodeConflict           = response.ErrCodeConflict
	ErrCodeInvalidInput       = response.ErrCodeInvalidInput
	ErrCodeRateLimited        = response.ErrCodeRateLimited
	ErrCodeAIUnavailable      = response.ErrCodeAIUnavailable
	ErrCodeInternalError      = response.ErrCodeInternalError
	ErrCodeServiceUnavailable = response.ErrCodeServiceUnavailable
)

func JSON(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	response.JSON(w, r, statusCode, data)
}

func ErrorJSON(w http.ResponseWriter, r *http.Request, statusCode int, code, message string, details any) {
	response.ErrorJSON(w, r, statusCode, code, message, details)
}

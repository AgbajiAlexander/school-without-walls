package tenants

import (
	"net/mail"
	"strings"
	"unicode"
)

type StaffInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func validateStaffInvitationInput(
	slug string,
	request StaffInvitationRequest,
) (string, StaffInvitationRequest, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.Role = strings.TrimSpace(request.Role)

	if len(slug) == 0 || len(slug) > 80 ||
		!slugPattern.MatchString(slug) {
		return "", StaffInvitationRequest{}, ErrInvalidInput
	}

	if request.Role != "SCHOOL_ADMIN" &&
		request.Role != "PRINCIPAL" {
		return "", StaffInvitationRequest{}, ErrInvalidInput
	}

	if len(request.Email) == 0 ||
		len(request.Email) > 254 ||
		strings.IndexFunc(request.Email, unicode.IsControl) >= 0 {
		return "", StaffInvitationRequest{}, ErrInvalidInput
	}

	address, err := mail.ParseAddress(request.Email)
	if err != nil || address.Address != request.Email {
		return "", StaffInvitationRequest{}, ErrInvalidInput
	}

	return slug, request, nil
}

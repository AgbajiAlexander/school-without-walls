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
	request.Role = strings.TrimSpace(request.Role)

	if len(slug) == 0 || len(slug) > 80 ||
		!slugPattern.MatchString(slug) {
		return "", StaffInvitationRequest{}, ErrInvalidInput
	}

	if request.Role != "SCHOOL_ADMIN" &&
		request.Role != "PRINCIPAL" {
		return "", StaffInvitationRequest{}, ErrInvalidInput
	}

	email, err := normalizeInvitationEmail(request.Email)
	if err != nil {
		return "", StaffInvitationRequest{}, err
	}
	request.Email = email
	return slug, request, nil
}

func normalizeInvitationEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) == 0 ||
		len(email) > 254 ||
		strings.IndexFunc(email, unicode.IsControl) >= 0 {
		return "", ErrInvalidInput
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", ErrInvalidInput
	}
	return email, nil
}

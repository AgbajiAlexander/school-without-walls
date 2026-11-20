package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// StaffInvitationEmail creates a role-specific invitation message.
func StaffInvitationEmail(
	publicWebURL, token, tenantName, role string,
	expiresAt time.Time,
) (string, string, error) {
	var roleName string
	switch role {
	case "SCHOOL_ADMIN":
		roleName = "School Administrator"
	case "PRINCIPAL":
		roleName = "Principal"
	default:
		return "", "", errors.New("unsupported staff invitation role")
	}

	// School names are editable. Keep their whitespace from creating mail header lines.
	tenantName = strings.Join(strings.Fields(tenantName), " ")
	if strings.TrimSpace(token) == "" || tenantName == "" {
		return "", "", errors.New("invalid invitation details")
	}

	base, err := url.Parse(strings.TrimRight(publicWebURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" ||
		(base.Scheme != "https" && base.Scheme != "http") ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", "", errors.New("invalid public web URL")
	}

	invitationURL := strings.TrimRight(base.String(), "/") +
		"/invite/" + url.PathEscape(token)

	subject := "Invitation to join " + tenantName +
		" on School Without Walls"

	body := "You have been invited to join " + tenantName +
		" as a " + roleName + ".\r\n\r\n" +
		"Accept your invitation using this one-time link:\r\n" +
		invitationURL + "\r\n\r\n" +
		"This invitation expires at " +
		expiresAt.UTC().Format(time.RFC3339) + ".\r\n\r\n" +
		"If you were not expecting this invitation, " +
		"you can ignore this email.\r\n"

	return subject, body, nil
}

// SendStaffInvitation sends the staff message through existing SMTP transport.
func (m *SMTPMailer) SendStaffInvitation(
	ctx context.Context,
	email, token, tenantName, role string,
	expiresAt time.Time,
) error {
	subject, body, err := StaffInvitationEmail(
		m.publicWebURL, token, tenantName, role, expiresAt,
	)
	if err != nil {
		return err
	}

	return m.send(ctx, email, subject, body)
}

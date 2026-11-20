package tenants

import (
	"context"
	"time"

	"github.com/school-without-walls/api/internal/modules/auth"
)

// SendStaffInvitation records a staff-specific invitation in the
// local development mail capture. It does not send an external email.
func (m *CaptureMailer) SendStaffInvitation(
	_ context.Context,
	email, token, schoolName, role string,
	expiresAt time.Time,
) error {
	subject, body, err := auth.StaffInvitationEmail(
		m.baseURL, token, schoolName, role, expiresAt,
	)
	if err != nil {
		return err
	}

	message := CaptureMessage{
		To:        email,
		Subject:   subject,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}

	m.mu.Lock()
	m.messages = append(m.messages, message)
	m.mu.Unlock()

	return nil
}

// Verify both production SMTP and local capture satisfy the
// Staff Invitation delivery interface.
var _ StaffInvitationMailer = (*auth.SMTPMailer)(nil)
var _ StaffInvitationMailer = (*CaptureMailer)(nil)

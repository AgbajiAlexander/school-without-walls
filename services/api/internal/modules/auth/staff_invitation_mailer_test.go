package auth

import (
	"strings"
	"testing"
	"time"
)

func TestStaffInvitationEmail(t *testing.T) {
	expires := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		role     string
		roleName string
	}{
		{"SCHOOL_ADMIN", "School Administrator"},
		{"PRINCIPAL", "Principal"},
	} {
		t.Run(tc.role, func(t *testing.T) {
			subject, body, err := StaffInvitationEmail(
				"http://localhost:3000",
				"example-token",
				"Morfol Demo School",
				tc.role,
				expires,
			)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(subject, "Morfol Demo School") {
				t.Fatalf("incorrect subject: %q", subject)
			}
			for _, expected := range []string{
				tc.roleName,
				"http://localhost:3000/invite/example-token",
				expires.Format(time.RFC3339),
			} {
				if !strings.Contains(body, expected) {
					t.Errorf("message does not contain %q", expected)
				}
			}
			if strings.Contains(body, "as its school owner") {
				t.Fatal("staff invitation contains owner onboarding language")
			}
		})
	}

	t.Run("rejects school owner role", func(t *testing.T) {
		_, _, err := StaffInvitationEmail(
			"http://localhost:3000",
			"token",
			"Morfol Demo School",
			"SCHOOL_OWNER",
			expires,
		)
		if err == nil {
			t.Fatal("expected rejection")
		}
	})

	t.Run("school name cannot introduce mail header lines", func(t *testing.T) {
		subject, body, err := StaffInvitationEmail(
			"http://localhost:3000", "fictional-token", "Fictional\r\nSchool", "PRINCIPAL", expires,
		)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(subject, "\r\n") || !strings.Contains(subject, "Fictional School") ||
			!strings.Contains(body, "Fictional School") {
			t.Fatal("expected a single-line school name in the staff invitation")
		}
	})

	t.Run("rejects invalid public URL", func(t *testing.T) {
		_, _, err := StaffInvitationEmail(
			"not-a-valid-url",
			"token",
			"Morfol Demo School",
			"PRINCIPAL",
			expires,
		)
		if err == nil {
			t.Fatal("expected rejection")
		}
	})
}

package tenants

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStaffInvitationCaptureMailer(t *testing.T) {
	ctx := context.Background()
	expires := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

	mailer := NewCaptureMailer("http://localhost:3000")

	tests := []struct {
		email string
		role  string
		label string
	}{
		{
			email: "admin@school.example",
			role:  "SCHOOL_ADMIN",
			label: "School Administrator",
		},
		{
			email: "principal@school.example",
			role:  "PRINCIPAL",
			label: "Principal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			token := "test-token-" + tc.role
			err := mailer.SendStaffInvitation(
				ctx, tc.email, token,
				"Morfol Demo School", tc.role, expires,
			)
			if err != nil {
				t.Fatal(err)
			}

			messages := mailer.Messages()
			message := messages[len(messages)-1]

			if message.To != tc.email {
				t.Fatalf("unexpected recipient: %q", message.To)
			}
			for _, expected := range []string{
				"Morfol Demo School",
				tc.label,
				"http://localhost:3000/invite/" + token,
				expires.Format(time.RFC3339),
			} {
				if !strings.Contains(message.Subject+"\n"+message.Body, expected) {
					t.Errorf("captured message missing %q", expected)
				}
			}

			if strings.Contains(message.Body, "as its school owner") {
				t.Fatal("staff invitation contains owner onboarding text")
			}
		})
	}

	t.Run("unauthorized role is rejected", func(t *testing.T) {
		before := len(mailer.Messages())

		err := mailer.SendStaffInvitation(
			ctx,
			"owner@school.example",
			"unauthorized-token",
			"Morfol Demo School",
			"SCHOOL_OWNER",
			expires,
		)
		if err == nil {
			t.Fatal("expected unsupported role error")
		}
		if len(mailer.Messages()) != before {
			t.Fatal("invalid role unexpectedly captured an email")
		}
	})

	t.Run("existing owner invitation remains unchanged", func(t *testing.T) {
		err := mailer.SendInvitation(
			ctx,
			"owner@school.example",
			"existing-owner-token",
			"Morfol Demo School",
			expires,
		)
		if err != nil {
			t.Fatal(err)
		}

		messages := mailer.Messages()
		message := messages[len(messages)-1]

		if message.Subject !=
			"Activate your School Without Walls school owner account" {
			t.Fatalf("owner email subject changed: %q", message.Subject)
		}

		if !strings.Contains(message.Body, "as its school owner") {
			t.Fatal("original owner invitation wording missing")
		}
	})
}

package tenants

import (
	"errors"
	"testing"
)

func TestValidateStaffInvitationInput(t *testing.T) {
	tests := []struct {
		name      string
		slug      string
		email     string
		role      string
		wantErr   bool
		wantSlug  string
		wantEmail string
	}{
		{
			name:      "valid school admin",
			slug:      "morfol-demo",
			email:     "admin@school.example",
			role:      "SCHOOL_ADMIN",
			wantSlug:  "morfol-demo",
			wantEmail: "admin@school.example",
		},
		{
			name:      "valid principal",
			slug:      "morfol-demo",
			email:     "principal@school.example",
			role:      "PRINCIPAL",
			wantSlug:  "morfol-demo",
			wantEmail: "principal@school.example",
		},
		{
			name:      "normalizes school and email",
			slug:      " MORFOL-DEMO ",
			email:     " Admin@School.Example ",
			role:      "SCHOOL_ADMIN",
			wantSlug:  "morfol-demo",
			wantEmail: "admin@school.example",
		},
		{
			name:    "reject school owner",
			slug:    "morfol-demo",
			email:   "owner@school.example",
			role:    "SCHOOL_OWNER",
			wantErr: true,
		},
		{
			name:    "reject platform administrator",
			slug:    "morfol-demo",
			email:   "admin@school.example",
			role:    "PLATFORM_ADMIN",
			wantErr: true,
		},
		{
			name:    "reject teacher role",
			slug:    "morfol-demo",
			email:   "teacher@school.example",
			role:    "TEACHER",
			wantErr: true,
		},
		{
			name:    "reject invalid email",
			slug:    "morfol-demo",
			email:   "not-an-email",
			role:    "SCHOOL_ADMIN",
			wantErr: true,
		},
		{
			name:    "reject display name email",
			slug:    "morfol-demo",
			email:   "Admin <admin@school.example>",
			role:    "SCHOOL_ADMIN",
			wantErr: true,
		},
		{
			name:    "reject empty email",
			slug:    "morfol-demo",
			email:   "",
			role:    "PRINCIPAL",
			wantErr: true,
		},
		{
			name:    "reject invalid school slug",
			slug:    "morfol demo",
			email:   "admin@school.example",
			role:    "SCHOOL_ADMIN",
			wantErr: true,
		},
		{
			name:    "reject empty school slug",
			slug:    "",
			email:   "admin@school.example",
			role:    "SCHOOL_ADMIN",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, request, err := validateStaffInvitationInput(
				tt.slug,
				StaffInvitationRequest{
					Email: tt.email,
					Role:  tt.role,
				},
			)

			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("expected ErrInvalidInput, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if slug != tt.wantSlug {
				t.Errorf("slug = %q, want %q", slug, tt.wantSlug)
			}

			if request.Email != tt.wantEmail {
				t.Errorf("email = %q, want %q", request.Email, tt.wantEmail)
			}

			if request.Role != tt.role {
				t.Errorf("role = %q, want %q", request.Role, tt.role)
			}
		})
	}
}

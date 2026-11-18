package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/academics"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/modules/tenants"
)

// The embedded interface supplies methods irrelevant to these route-security tests.
// These methods count calls to verify middleware rejects requests before repository access.
type assignmentSecurityRepo struct {
	academics.TeacherAssignmentRepository
	calls int
}

func (r *assignmentSecurityRepo) CreateTeacherAssignment(
	_ context.Context, _ string, _ uuid.UUID,
	_ academics.CreateTeacherAssignmentRequest, _ string,
) (*academics.TeacherAssignment, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *assignmentSecurityRepo) ListTeacherAssignments(
	_ context.Context, _ string, _ uuid.UUID,
) ([]academics.TeacherAssignment, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *assignmentSecurityRepo) DeactivateTeacherAssignment(
	_ context.Context, _ string, _, _ uuid.UUID, _ string,
) (*academics.TeacherAssignment, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func TestTeacherAssignmentRoutesRequireAuthCSRFAndRepositoryAuthorization(t *testing.T) {
	authRepo := auth.NewMockRepository()
	password := "a sufficiently long fictional assignment password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := &auth.User{
		ID:           uuid.New(),
		Email:        "assignment-route-user@school.invalid",
		PasswordHash: hash,
		FullName:     "Fictional Assignment Tester",
		Status:       auth.UserStatusActive,
	}
	if err := authRepo.CreateUser(t.Context(), user); err != nil {
		t.Fatal(err)
	}

	repo := &assignmentSecurityRepo{}
	router, err := NewRouter(RouterConfig{
		Config: &config.Config{
			Environment:        "test",
			PublicWebURL:       "http://localhost:3000",
			CORSAllowedOrigins: []string{"http://localhost:3000"},
		},
		DB:             &mockPinger{},
		AuthRepo:       authRepo,
		TenantRepo:     tenants.NewMockRepository(),
		AcademicRepo:   academics.NewMockRepository(),
		AssignmentRepo: repo,
	})
	if err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(authRepo)
	_, sessionToken, csrfToken, err := authSvc.Login(
		t.Context(), user.Email, password,
		"127.0.0.1", "test-agent", "assignment-login",
	)
	if err != nil {
		t.Fatal(err)
	}

	id := uuid.NewString()
	base := "/api/v1/tenants/school-a/teacher-assignments"
	body := `{"offering_id":"` + uuid.NewString() + `","teacher_membership_id":"` + uuid.NewString() + `"}`
	cases := []struct {
		name, method, path, body string
		mutation                 bool
	}{
		{"list assignments", http.MethodGet, base, "", false},
		{"create assignment", http.MethodPost, base, body, true},
		{"deactivate assignment", http.MethodPost, base + "/" + id + "/deactivate", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			send := func(authenticated, withCSRF bool) *httptest.ResponseRecorder {
				request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				request.Header.Set("Origin", "http://localhost:3000")
				request.Header.Set("Content-Type", "application/json")
				if authenticated {
					request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
				}
				if withCSRF {
					request.Header.Set(auth.HeaderCSRF, csrfToken)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				return response
			}

			repo.calls = 0
			if response := send(false, false); response.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous request: status=%d body=%s", response.Code, response.Body.String())
			}
			if repo.calls != 0 {
				t.Fatalf("anonymous request reached repository: %d calls", repo.calls)
			}
			if tc.mutation {
				if response := send(true, false); response.Code != http.StatusForbidden {
					t.Fatalf("missing CSRF: status=%d body=%s", response.Code, response.Body.String())
				}
				if repo.calls != 0 {
					t.Fatalf("missing CSRF request reached repository: %d calls", repo.calls)
				}
			}
			if response := send(true, true); response.Code != http.StatusForbidden {
				t.Fatalf("repository denial: status=%d body=%s", response.Code, response.Body.String())
			}
			if repo.calls != 1 {
				t.Fatalf("expected exactly one repository call after valid auth, got %d", repo.calls)
			}
		})
	}
}

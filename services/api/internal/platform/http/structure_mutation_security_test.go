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

// The embedded interface supplies methods irrelevant to these tests.
// Every mutation method is implemented explicitly below.
type mutationRouteRepo struct {
	academics.StructureRepository
	calls int
}

func (r *mutationRouteRepo) UpdateGradeLevel(
	_ context.Context, _ string, _, _ uuid.UUID,
	_ academics.UpdateGradeLevelRequest, _ string,
) (*academics.GradeLevel, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *mutationRouteRepo) ArchiveGradeLevel(
	_ context.Context, _ string, _, _ uuid.UUID, _ string,
) (*academics.GradeLevel, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *mutationRouteRepo) UpdateClass(
	_ context.Context, _ string, _, _ uuid.UUID,
	_ academics.UpdateClassRequest, _ string,
) (*academics.Class, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *mutationRouteRepo) ArchiveClass(
	_ context.Context, _ string, _, _ uuid.UUID, _ string,
) (*academics.Class, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *mutationRouteRepo) UpdateClassArm(
	_ context.Context, _ string, _, _ uuid.UUID,
	_ academics.UpdateClassArmRequest, _ string,
) (*academics.ClassArm, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *mutationRouteRepo) ArchiveClassArm(
	_ context.Context, _ string, _, _ uuid.UUID, _ string,
) (*academics.ClassArm, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func TestStructureMutationRoutesRequireAuthAndCSRF(t *testing.T) {
	authRepo := auth.NewMockRepository()
	password := "a sufficiently long fictional mutation password"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	user := &auth.User{
		ID:           uuid.New(),
		Email:        "mutation-route-user@school.invalid",
		PasswordHash: hash,
		FullName:     "Fictional Mutation Tester",
		Status:       auth.UserStatusActive,
	}
	if err := authRepo.CreateUser(t.Context(), user); err != nil {
		t.Fatal(err)
	}

	repo := &mutationRouteRepo{}
	router, err := NewRouter(RouterConfig{
		Config: &config.Config{
			Environment:        "test",
			PublicWebURL:       "http://localhost:3000",
			CORSAllowedOrigins: []string{"http://localhost:3000"},
		},
		DB:            &mockPinger{},
		AuthRepo:      authRepo,
		TenantRepo:    tenants.NewMockRepository(),
		AcademicRepo:  academics.NewMockRepository(),
		StructureRepo: repo,
	})
	if err != nil {
		t.Fatal(err)
	}

	authSvc := auth.NewService(authRepo)
	_, sessionToken, csrfToken, err := authSvc.Login(
		t.Context(), user.Email, password,
		"127.0.0.1", "test-agent", "mutation-login",
	)
	if err != nil {
		t.Fatal(err)
	}

	id := uuid.NewString()
	base := "/api/v1/tenants/school-a"

	cases := []struct {
		name, method, path, body string
	}{
		{"update grade level", http.MethodPatch, base + "/grade-levels/" + id, `{"name":"Updated Grade"}`},
		{"archive grade level", http.MethodPost, base + "/grade-levels/" + id + "/archive", ""},
		{"update class", http.MethodPatch, base + "/classes/" + id, `{"name":"Updated Class"}`},
		{"archive class", http.MethodPost, base + "/classes/" + id + "/archive", ""},
		{"update class arm", http.MethodPatch, base + "/class-arms/" + id, `{"name":"Updated Arm"}`},
		{"archive class arm", http.MethodPost, base + "/class-arms/" + id + "/archive", ""},
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

			if response := send(true, false); response.Code != http.StatusForbidden {
				t.Fatalf("missing CSRF: status=%d body=%s", response.Code, response.Body.String())
			}

			if repo.calls != 0 {
				t.Fatalf("repository invoked before authentication/CSRF passed: %d calls", repo.calls)
			}

			if response := send(true, true); response.Code != http.StatusForbidden {
				t.Fatalf("repository denial: status=%d body=%s", response.Code, response.Body.String())
			}

			if repo.calls != 1 {
				t.Fatalf("expected exactly one repository call, got %d", repo.calls)
			}
		})
	}
}

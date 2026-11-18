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

// The embedded interface provides irrelevant methods. The four routes being
// tested are implemented explicitly and fail closed when invoked.
type deniedSubjectRouteRepo struct {
	academics.SubjectRepository
	calls int
}

func (r *deniedSubjectRouteRepo) CreateSubject(_ context.Context, _ string, _ uuid.UUID, _ academics.CreateSubjectRequest, _ string) (*academics.Subject, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *deniedSubjectRouteRepo) ListSubjects(_ context.Context, _ string, _ uuid.UUID) ([]academics.Subject, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *deniedSubjectRouteRepo) UpdateSubject(_ context.Context, _ string, _, _ uuid.UUID, _ academics.UpdateSubjectRequest, _ string) (*academics.Subject, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func (r *deniedSubjectRouteRepo) ArchiveSubject(_ context.Context, _ string, _, _ uuid.UUID, _ string) (*academics.Subject, error) {
	r.calls++
	return nil, academics.ErrForbidden
}

func TestSubjectRoutesRequireAuthCSRFAndRepositoryAuthorization(t *testing.T) {
	authRepo := auth.NewMockRepository()
	password := "a sufficiently long fictional subject security password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	user := &auth.User{
		ID: uuid.New(), Email: "subject-http-user@school.invalid",
		PasswordHash: hash, FullName: "Fictional Subject HTTP Tester",
		Status: auth.UserStatusActive,
	}
	if err := authRepo.CreateUser(t.Context(), user); err != nil {
		t.Fatal(err)
	}

	repo := &deniedSubjectRouteRepo{}
	router, err := NewRouter(RouterConfig{
		Config: &config.Config{
			Environment: "test", PublicWebURL: "http://localhost:3000",
			CORSAllowedOrigins: []string{"http://localhost:3000"},
		},
		DB:           &mockPinger{},
		AuthRepo:     authRepo,
		TenantRepo:   tenants.NewMockRepository(),
		AcademicRepo: academics.NewMockRepository(),
		SubjectRepo:  repo,
	})
	if err != nil {
		t.Fatal(err)
	}

	authSvc := auth.NewService(authRepo)
	_, sessionToken, csrfToken, err := authSvc.Login(
		t.Context(), user.Email, password, "127.0.0.1", "test-agent", "subject-http-login",
	)
	if err != nil {
		t.Fatal(err)
	}

	base := "/api/v1/tenants/school-a/subjects"
	id := uuid.NewString()
	cases := []struct {
		name, method, path, body string
		mutation                 bool
	}{
		{"list subjects", http.MethodGet, base, "", false},
		{"create subject", http.MethodPost, base, `{"code":"MATH","name":"Mathematics"}`, true},
		{"update subject", http.MethodPatch, base + "/" + id, `{"name":"Updated Mathematics"}`, true},
		{"archive subject", http.MethodPost, base + "/" + id + "/archive", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			send := func(authenticated, withCSRF bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Origin", "http://localhost:3000")
				req.Header.Set("Content-Type", "application/json")
				if authenticated {
					req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
				}
				if withCSRF {
					req.Header.Set(auth.HeaderCSRF, csrfToken)
				}
				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, req)
				return resp
			}

			repo.calls = 0
			if resp := send(false, false); resp.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous request: status=%d body=%s", resp.Code, resp.Body.String())
			}
			if repo.calls != 0 {
				t.Fatalf("repository called for anonymous request: %d", repo.calls)
			}
			if tc.mutation {
				if resp := send(true, false); resp.Code != http.StatusForbidden {
					t.Fatalf("missing CSRF: status=%d body=%s", resp.Code, resp.Body.String())
				}
				if repo.calls != 0 {
					t.Fatalf("repository called without CSRF: %d", repo.calls)
				}
			}
			if resp := send(true, tc.mutation); resp.Code != http.StatusForbidden {
				t.Fatalf("repository denial: status=%d body=%s", resp.Code, resp.Body.String())
			}
			if repo.calls != 1 {
				t.Fatalf("expected exactly one repository call after authorization, got %d", repo.calls)
			}
		})
	}
}

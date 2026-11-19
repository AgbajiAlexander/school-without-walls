package tenants

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

func TestListMineRequiresAuthentication(t *testing.T) {
	repo := NewMockRepository()
	handler := NewHandler(NewService(repo, nil), nil)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/my/schools",
		nil,
	)
	response := httptest.NewRecorder()

	handler.ListMine(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected 401, got %d: %s",
			response.Code,
			response.Body.String(),
		)
	}
}

func TestListMineDoesNotDiscloseOtherSchools(t *testing.T) {
	repo := NewMockRepository()
	userA := uuid.New()
	userB := uuid.New()

	repo.Tenants["school-a"] = &Tenant{
		ID:     uuid.New(),
		Slug:   "school-a",
		Name:   "School A",
		Status: "ACTIVE",
	}
	repo.Tenants["school-b"] = &Tenant{
		ID:     uuid.New(),
		Slug:   "school-b",
		Name:   "School B",
		Status: "ACTIVE",
	}

	repo.AddMembership("school-a", userA, "SCHOOL_OWNER")
	repo.AddMembership("school-b", userB, "SCHOOL_OWNER")

	handler := NewHandler(NewService(repo, nil), nil)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/my/schools",
		nil,
	)
	request = request.WithContext(
		context.WithValue(
			request.Context(),
			auth.UserKey,
			&auth.User{ID: userA},
		),
	)

	response := httptest.NewRecorder()
	handler.ListMine(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			response.Code,
			response.Body.String(),
		)
	}

	body := response.Body.String()

	if !strings.Contains(body, "school-a") {
		t.Fatal("authorized school missing from response")
	}

	if strings.Contains(body, "school-b") {
		t.Fatal("cross-school information disclosed")
	}
}

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

func TestProvisionRejectsClientSelectedRole(t *testing.T) {
	repo := NewMockRepository()
	actorID := uuid.New()
	repo.PlatformAdmins[actorID] = true
	mailer := NewCaptureMailer("http://localhost:3000")
	handler := NewHandler(NewService(repo, mailer), mailer)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/tenants",
		strings.NewReader(`{"slug":"demo-school","name":"Demo School","owner_email":"owner@school.invalid","role":"PLATFORM_ADMIN"}`))
	request = request.WithContext(context.WithValue(request.Context(), auth.UserKey, &auth.User{ID: actorID}))
	response := httptest.NewRecorder()

	handler.Provision(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("client-supplied role should be rejected, status=%d body=%s", response.Code, response.Body.String())
	}
	if len(repo.Tenants) != 0 || len(mailer.Messages()) != 0 {
		t.Fatal("invalid request must not provision tenant or send invitation")
	}
}

func TestProvisionRequiresAuthenticatedActor(t *testing.T) {
	handler := NewHandler(NewService(NewMockRepository(), NewCaptureMailer("http://localhost:3000")), nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/tenants", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	handler.Provision(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated response, got %d", response.Code)
	}
}

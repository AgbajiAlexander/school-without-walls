package tenants

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestListMineReturnsOnlyMembershipSchools(t *testing.T) {
	repo := NewMockRepository()
	actorID := uuid.New()
	otherID := uuid.New()

	repo.Tenants["morfol-demo"] = &Tenant{
		Slug:   "morfol-demo",
		Name:   "Morfol Demo School",
		Status: "ACTIVE",
	}
	repo.Tenants["other-school"] = &Tenant{
		Slug:   "other-school",
		Name:   "Other School",
		Status: "ACTIVE",
	}
	repo.Tenants["archived-school"] = &Tenant{
		Slug:   "archived-school",
		Name:   "Archived School",
		Status: "ARCHIVED",
	}

	repo.AddMembership("morfol-demo", actorID, "SCHOOL_OWNER")
	repo.AddMembership("other-school", otherID, "SCHOOL_OWNER")
	repo.AddMembership("archived-school", actorID, "SCHOOL_OWNER")

	service := NewService(repo, nil)

	schools, err := service.ListMine(context.Background(), actorID)
	if err != nil {
		t.Fatalf("list member schools: %v", err)
	}
	if len(schools) != 1 {
		t.Fatalf("expected one accessible school, got %d", len(schools))
	}
	if schools[0].Slug != "morfol-demo" {
		t.Fatalf("unexpected school: %s", schools[0].Slug)
	}
	if schools[0].CurrentRole != "SCHOOL_OWNER" {
		t.Fatalf("unexpected membership role: %s", schools[0].CurrentRole)
	}
}

func TestListMineReturnsEmptyForUserWithoutMembership(t *testing.T) {
	repo := NewMockRepository()
	repo.Tenants["morfol-demo"] = &Tenant{
		Slug:   "morfol-demo",
		Name:   "Morfol Demo School",
		Status: "ACTIVE",
	}

	service := NewService(repo, nil)
	schools, err := service.ListMine(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("list member schools: %v", err)
	}
	if len(schools) != 0 {
		t.Fatalf("expected no accessible schools, got %d", len(schools))
	}
}

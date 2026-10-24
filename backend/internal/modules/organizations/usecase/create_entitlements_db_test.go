package usecase

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCreateWithEntitlements: Takwim yönetimi opens a business with its
// sector, plan, add-ons and access end in one step (TEC-927).
func TestCreateWithEntitlements(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	orgIDs := []int64{}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = ANY($1)", orgIDs)
	})
	repo := repository.NewPostgres(pool, q)
	svc := New(repo, branchusecase.NewOrganizationHooks(nil))
	mgmt := NewManagement(svc, repo, ManagementConfig{FrontendURL: "https://app.example.test/"})
	actor := Actor{Name: "Platform"}

	ends := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	org, _, err := mgmt.CreateOrganizationWithOwnerInvitation(ctx, orgmodel.CreateOrganizationInput{
		Name: "Klinik " + suffix, Sector: "clinic", Plan: "growth",
		Addons: []string{"confirmation", "confirmation"}, AccessEndsAt: &ends,
	}, "klinik-"+suffix+"@example.test", actor)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	orgIDs = append(orgIDs, org.ID)
	if org.Sector != "clinic" || org.Plan != "growth" || len(org.Addons) != 1 || org.Addons[0] != "confirmation" ||
		org.AccessEndsAt == nil || !org.AccessEndsAt.Equal(ends) {
		t.Fatalf("created = %+v", org)
	}

	// Add-ons alone keep the default plan.
	org, _, err = mgmt.CreateOrganizationWithOwnerInvitation(ctx, orgmodel.CreateOrganizationInput{
		Name: "Salon " + suffix, Addons: []string{"confirmation"},
	}, "salon-"+suffix+"@example.test", actor)
	if err != nil {
		t.Fatalf("create add-on only: %v", err)
	}
	orgIDs = append(orgIDs, org.ID)
	if org.Plan != "starter" || len(org.Addons) != 1 || org.AccessEndsAt != nil {
		t.Fatalf("add-on only = %+v", org)
	}

	past := time.Now().Add(-time.Hour)
	for name, in := range map[string]orgmodel.CreateOrganizationInput{
		"plan":   {Name: "X " + suffix, Plan: "gold"},
		"addon":  {Name: "Y " + suffix, Addons: []string{"fax"}},
		"access": {Name: "Z " + suffix, AccessEndsAt: &past},
	} {
		if _, _, err := mgmt.CreateOrganizationWithOwnerInvitation(ctx, in, "bad-"+suffix+"@example.test", actor); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

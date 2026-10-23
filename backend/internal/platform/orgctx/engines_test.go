package orgctx

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestEngineScope(t *testing.T) {
	t.Parallel()
	platform := context.Background()
	tenant := WithScope(platform, Scope{ID: 7, Slug: "acme"})

	if org, err := EngineScope(platform, "platform.users"); err != nil || org.Valid {
		t.Fatalf("platform resource outside org = %v, %v", org, err)
	}
	if _, err := EngineScope(platform, "tenant.branches"); err == nil {
		t.Fatal("tenant resource without an organization must fail")
	}
	if _, err := EngineScope(tenant, "platform.users"); err == nil {
		t.Fatal("platform resource inside an organization must fail")
	}
	if org, err := EngineScope(tenant, "tenant.branches"); err != nil || !org.Valid || org.Int64 != 7 {
		t.Fatalf("tenant resource in org = %v, %v", org, err)
	}

	if !SameEngineScope(platform, pgtype.Int8{}) || SameEngineScope(platform, pgtype.Int8{Int64: 7, Valid: true}) {
		t.Fatal("platform context must see platform jobs only")
	}
	if !SameEngineScope(tenant, pgtype.Int8{Int64: 7, Valid: true}) ||
		SameEngineScope(tenant, pgtype.Int8{Int64: 8, Valid: true}) ||
		SameEngineScope(tenant, pgtype.Int8{}) {
		t.Fatal("tenant context must see its own organization's jobs only")
	}

	job := WithOrganizationID(platform, 9)
	if id, ok := OrganizationID(job); !ok || id != 9 {
		t.Fatalf("worker scope = %d, %v", id, ok)
	}
	if WithOrganizationID(tenant, 7) != tenant {
		t.Fatal("matching request scope must be kept")
	}
}

package entitlement

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

func TestResolveDefaultsAndOverrides(t *testing.T) {
	e := Resolve("hairdresser", Stored{})
	if e.Plan != PlanStarter || e.BranchLimit == nil || *e.BranchLimit != 1 || *e.StaffLimit != 5 {
		t.Fatalf("starter defaults: %+v", e)
	}
	if e.Allows(sector.ModuleConfirmation) {
		t.Fatal("confirmation needs the add-on")
	}
	if !e.Allows(sector.ModuleStaffShares) || !e.Allows(sector.ModuleReservations) {
		t.Fatal("sector modules stay visible")
	}
	three := int32(3)
	e = Resolve("restaurant", Stored{Plan: "enterprise", Addons: []string{"confirmation", "bogus"}, StaffLimit: &three})
	if e.BranchLimit != nil || e.StaffLimit == nil || *e.StaffLimit != 3 {
		t.Fatalf("enterprise + override: %+v", e)
	}
	if !e.Allows(sector.ModuleConfirmation) || !e.HasAddon(AddonConfirmation) || len(e.Addons) != 1 {
		t.Fatalf("granted add-on: %+v", e)
	}
	if e.Allows(sector.ModuleStaffShares) {
		t.Fatal("entitlements never show a module the sector hides")
	}
	if Resolve("gym", Stored{Plan: "gold"}).Plan != DefaultPlan {
		t.Fatal("unknown plan falls back to the default")
	}
}

func TestCheckAdd(t *testing.T) {
	one := int32(1)
	if err := CheckAdd(LimitBranches, nil, 1000); err != nil {
		t.Fatalf("unlimited: %v", err)
	}
	if err := CheckAdd(LimitBranches, &one, 0); err != nil {
		t.Fatalf("first fits: %v", err)
	}
	err := CheckAdd(LimitBranches, &one, 1)
	var le *LimitError
	if !errors.Is(err, ErrLimitReached) || !errors.As(err, &le) || le.Kind != LimitBranches || le.Limit != 1 || le.Used != 1 {
		t.Fatalf("second over: %v", err)
	}
	if !ValidPlan("growth") || ValidPlan("") || !ValidAddon("confirmation") || ValidAddon("sms") {
		t.Fatal("catalog validation")
	}
}

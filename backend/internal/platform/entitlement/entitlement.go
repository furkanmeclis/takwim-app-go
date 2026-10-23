// Package entitlement holds what a business may use (TEC-883): its plan
// (branch and staff limits), the add-on modules granted on top of the plan
// (Konfirmasyon) and the monthly communication usage counters.
//
// Visibility has two layers: the sector decides which modules exist for a
// business (internal/platform/sector); entitlements narrow that set (an
// add-on module stays hidden until it is granted). Entitlements never show
// a module the sector hides.
//
// Plans are assigned by Takwim yönetimi (the platform). Prices, trials and
// automatic subscription billing are out of scope (a separate commercial
// decision): a plan here is only a set of default limits.
package entitlement

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// PlanCode identifies a plan (organizations.plan, CHECK in migration
// 000040).
type PlanCode string

// Plans.
const (
	PlanStarter    PlanCode = "starter"
	PlanGrowth     PlanCode = "growth"
	PlanEnterprise PlanCode = "enterprise"
)

// DefaultPlan is the plan of a business created without one.
const DefaultPlan = PlanStarter

// Plan is one plan's default limits. A nil limit means unlimited.
type Plan struct {
	Code        PlanCode `json:"code"`
	BranchLimit *int32   `json:"branch_limit"`
	StaffLimit  *int32   `json:"staff_limit"`
}

func limit(n int32) *int32 { return &n }

var plans = []Plan{
	{Code: PlanStarter, BranchLimit: limit(1), StaffLimit: limit(5)},
	{Code: PlanGrowth, BranchLimit: limit(3), StaffLimit: limit(20)},
	{Code: PlanEnterprise},
}

// Plans returns every plan in display order.
func Plans() []Plan {
	out := make([]Plan, len(plans))
	copy(out, plans)
	return out
}

// GetPlan returns a plan by code.
func GetPlan(code string) (Plan, bool) {
	for _, p := range plans {
		if string(p.Code) == code {
			return p, true
		}
	}
	return Plan{}, false
}

// ValidPlan reports whether code names a plan.
func ValidPlan(code string) bool {
	_, ok := GetPlan(code)
	return ok
}

// Addon is a module sold separately from the plan.
type Addon string

// Add-ons. Konfirmasyon is the one add-on of the beta (requirements §9).
const (
	AddonConfirmation Addon = "confirmation"
)

// Addons lists every add-on.
var Addons = []Addon{AddonConfirmation}

// ValidAddon reports whether code names an add-on.
func ValidAddon(code string) bool {
	for _, a := range Addons {
		if string(a) == code {
			return true
		}
	}
	return false
}

// addonModule maps an add-on to the sector module it unlocks.
var addonModule = map[Addon]sector.Module{
	AddonConfirmation: sector.ModuleConfirmation,
}

// Limit kinds.
const (
	LimitBranches = "branches"
	LimitStaff    = "staff"
)

// Effective is what a business may use right now.
type Effective struct {
	Plan PlanCode `json:"plan"`
	// Addons granted to the business.
	Addons []string `json:"addons"`
	// BranchLimit / StaffLimit: the business override, else the plan
	// default; nil = unlimited.
	BranchLimit *int32 `json:"branch_limit"`
	StaffLimit  *int32 `json:"staff_limit"`
	// Modules: the sector's modules the business may use (add-on modules
	// only when granted), in the sector's order.
	Modules []sector.Module `json:"modules"`
}

// Stored is what the organizations row holds.
type Stored struct {
	Plan        string
	Addons      []string
	BranchLimit *int32
	StaffLimit  *int32
}

// FromScope reads the stored entitlements of a request scope.
func FromScope(s orgctx.Scope) Stored {
	return Stored{Plan: s.Plan, Addons: s.Addons, BranchLimit: s.BranchLimit, StaffLimit: s.StaffLimit}
}

// Resolve computes the effective entitlements of a business of a sector.
// An unknown stored plan falls back to the default plan.
func Resolve(sectorCode string, st Stored) Effective {
	plan, ok := GetPlan(st.Plan)
	if !ok {
		plan, _ = GetPlan(string(DefaultPlan))
	}
	out := Effective{
		Plan: plan.Code, Addons: []string{},
		BranchLimit: plan.BranchLimit, StaffLimit: plan.StaffLimit,
	}
	if st.BranchLimit != nil {
		out.BranchLimit = st.BranchLimit
	}
	if st.StaffLimit != nil {
		out.StaffLimit = st.StaffLimit
	}
	granted := map[sector.Module]bool{}
	for _, a := range st.Addons {
		if ValidAddon(a) {
			out.Addons = append(out.Addons, a)
			granted[addonModule[Addon(a)]] = true
		}
	}
	gated := map[sector.Module]bool{}
	for _, m := range addonModule {
		gated[m] = true
	}
	def := sector.Of(sectorCode)
	out.Modules = make([]sector.Module, 0, len(def.Modules))
	for _, m := range def.Modules {
		if gated[m] && !granted[m] {
			continue
		}
		out.Modules = append(out.Modules, m)
	}
	return out
}

// Allows reports whether the business may use a module.
func (e Effective) Allows(m sector.Module) bool {
	for _, v := range e.Modules {
		if v == m {
			return true
		}
	}
	return false
}

// HasAddon reports whether an add-on is granted.
func (e Effective) HasAddon(a Addon) bool {
	for _, v := range e.Addons {
		if v == string(a) {
			return true
		}
	}
	return false
}

// ForScope resolves the effective entitlements of a request scope.
func ForScope(s orgctx.Scope) Effective {
	return Resolve(s.Sector, FromScope(s))
}

// ErrLimitReached: adding one more would exceed the plan limit.
var ErrLimitReached = errors.New("entitlement: plan limit reached")

// LimitError says which limit was reached.
type LimitError struct {
	Kind  string
	Limit int32
	Used  int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("entitlement: %s limit %d reached (%d in use)", e.Kind, e.Limit, e.Used)
}

// Unwrap lets errors.Is(err, ErrLimitReached) match.
func (e *LimitError) Unwrap() error { return ErrLimitReached }

// CheckAdd answers nil when used+1 fits the limit (nil limit = unlimited),
// else a *LimitError.
func CheckAdd(kind string, limit *int32, used int64) error {
	if limit == nil || used+1 <= int64(*limit) {
		return nil
	}
	return &LimitError{Kind: kind, Limit: *limit, Used: used}
}

// WriteLimitError answers 409 PLAN_LIMIT_REACHED with {kind, limit, used}
// when err is a plan limit error and reports whether it wrote.
func WriteLimitError(w http.ResponseWriter, r *http.Request, err error) bool {
	var le *LimitError
	if !errors.As(err, &le) {
		return false
	}
	response.ErrorWithData(w, r, http.StatusConflict, response.CodePlanLimitReached,
		"The plan limit is reached; ask Takwim to raise it", nil,
		map[string]any{"kind": le.Kind, "limit": le.Limit, "used": le.Used})
	return true
}

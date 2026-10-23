package bulkengine

import "context"

// BulkActionDef describes one bulk action exposed via resource meta.
type BulkActionDef struct {
	ID          string `json:"id"`
	LabelKey    string `json:"label_key"`
	Permission  string `json:"permission"`
	Destructive bool   `json:"destructive"`
	Reversible  bool   `json:"reversible"`
	ConfirmKey  string `json:"confirm_key,omitempty"`
}

// BulkTarget selects rows by explicit ids or list query filters. Params are
// the action arguments of parameterised actions (for example the new status
// of a "set status" action); adapters read them with Params(ctx) and validate
// them in ResolveTargets.
type BulkTarget struct {
	Scope  string            `json:"scope"` // ids | query
	IDs    []string          `json:"ids,omitempty"`
	Query  map[string]string `json:"query,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

type paramsKey struct{}

// WithParams stores the action params of a bulk job on ctx (set by the bulk
// service before ApplyItem runs).
func WithParams(ctx context.Context, params map[string]string) context.Context {
	return context.WithValue(ctx, paramsKey{}, params)
}

// Params returns the action params of the running bulk job (nil when none).
func Params(ctx context.Context) map[string]string {
	p, _ := ctx.Value(paramsKey{}).(map[string]string)
	return p
}

// BulkItemResult is the outcome of applying one bulk item. Skipped marks an
// item that was left as it is on purpose (the action does not apply in its
// current state); it is reported apart from failures. Label names the item
// in the result report (for example a reservation number); only labelled
// items are listed there.
type BulkItemResult struct {
	EntityUUID string
	EntityType string
	OK         bool
	Skipped    bool
	Error      string
	Label      string
	Op         string // update | delete
	Previous   map[string]any
}

// MaxSummaryIssues caps the per-item lines of a summary.
const MaxSummaryIssues = 200

// BulkIssue is one skipped or failed item of a summary.
type BulkIssue struct {
	EntityUUID string `json:"entity_uuid"`
	Label      string `json:"label,omitempty"`
	Error      string `json:"error"`
	Skipped    bool   `json:"skipped,omitempty"`
}

// BulkSummary aggregates job results. Issues lists the first
// MaxSummaryIssues skipped / failed items that carry a Label.
type BulkSummary struct {
	Total     int         `json:"total"`
	Succeeded int         `json:"succeeded"`
	Failed    int         `json:"failed"`
	Skipped   int         `json:"skipped,omitempty"`
	Issues    []BulkIssue `json:"issues,omitempty"`
}

// Add counts one item result into the summary.
func (s *BulkSummary) Add(res BulkItemResult) {
	switch {
	case res.OK:
		s.Succeeded++
		return
	case res.Skipped:
		s.Skipped++
	default:
		s.Failed++
	}
	// Only items the adapter named (it found them in the caller's reach)
	// are listed; an unknown uuid stays a bare count.
	if len(s.Issues) < MaxSummaryIssues && res.EntityUUID != "" && res.Label != "" {
		s.Issues = append(s.Issues, BulkIssue{EntityUUID: res.EntityUUID, Label: res.Label, Error: res.Error, Skipped: res.Skipped})
	}
}

// BulkAdapter connects bulkengine to a list resource.
type BulkAdapter interface {
	Resource() string
	BulkActions() []BulkActionDef
	ResolveTargets(ctx context.Context, action string, target BulkTarget) ([]string, error)
	ApplyItem(ctx context.Context, action, entityUUID string) (BulkItemResult, error)
	RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error
}

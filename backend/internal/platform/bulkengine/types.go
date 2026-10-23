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

// BulkTarget selects rows by explicit ids or list query filters.
type BulkTarget struct {
	Scope string            `json:"scope"` // ids | query
	IDs   []string          `json:"ids,omitempty"`
	Query map[string]string `json:"query,omitempty"`
}

// BulkItemResult is the outcome of applying one bulk item.
type BulkItemResult struct {
	EntityUUID string
	EntityType string
	OK         bool
	Error      string
	Op         string // update | delete
	Previous   map[string]any
}

// BulkSummary aggregates job results.
type BulkSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// BulkAdapter connects bulkengine to a list resource.
type BulkAdapter interface {
	Resource() string
	BulkActions() []BulkActionDef
	ResolveTargets(ctx context.Context, action string, target BulkTarget) ([]string, error)
	ApplyItem(ctx context.Context, action, entityUUID string) (BulkItemResult, error)
	RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error
}

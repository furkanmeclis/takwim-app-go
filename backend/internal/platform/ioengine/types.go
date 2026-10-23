package ioengine

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
)

// ColumnType describes export/import cell typing.
type ColumnType string

const (
	ColumnTypeString   ColumnType = "string"
	ColumnTypeEnum     ColumnType = "enum"
	ColumnTypeBoolean  ColumnType = "boolean"
	ColumnTypeUUID     ColumnType = "uuid"
	ColumnTypeDatetime ColumnType = "datetime"
)

// Column describes one export/import field.
type Column struct {
	Key      string     `json:"key"`
	LabelKey string     `json:"label_key"`
	Type     ColumnType `json:"type"`
	Required bool       `json:"required"`
}

// Dataset is format-agnostic tabular data.
type Dataset struct {
	Resource string           `json:"resource"`
	Columns  []Column         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
}

// ExportFormat supported for downloads.
type ExportFormat string

const (
	ExportPDF  ExportFormat = "pdf"
	ExportXLSX ExportFormat = "xlsx"
	ExportCSV  ExportFormat = "csv"
	ExportJSON ExportFormat = "json"
)

// ImportFormat supported for uploads.
type ImportFormat string

const (
	ImportJSON ImportFormat = "json"
	ImportXLSX ImportFormat = "xlsx"
	ImportCSV  ImportFormat = "csv"
	ImportTSV  ImportFormat = "tsv"
)

// ExportQuery carries list filters from the client.
type ExportQuery map[string]string

// ImportField describes an importable schema field.
type ImportField struct {
	Key         string     `json:"key"`
	LabelKey    string     `json:"label_key"`
	Type        ColumnType `json:"type"`
	Required    bool       `json:"required"`
	DefaultHint string     `json:"default_hint,omitempty"`
}

// RowResult is the outcome of applying one import row.
type RowResult struct {
	RowIndex   int            `json:"row_index"`
	OK         bool           `json:"ok"`
	Error      string         `json:"error,omitempty"`
	EntityType string         `json:"entity_type,omitempty"`
	EntityUUID string         `json:"entity_uuid,omitempty"`
	Op         string         `json:"op,omitempty"`
	Previous   map[string]any `json:"previous,omitempty"`
}

// PreviewSummary aggregates a dry-run. Creates / Updates are only counted
// for adapters that implement RowPreviewer.
type PreviewSummary struct {
	Total   int          `json:"total"`
	Valid   int          `json:"valid"`
	Invalid int          `json:"invalid"`
	Creates int          `json:"creates,omitempty"`
	Updates int          `json:"updates,omitempty"`
	Rows    []RowPreview `json:"rows"`
	Errors  []RowError   `json:"errors"`
}

// RowPreview is one previewed row. Op and Changes (the dry-run diff) are set
// for adapters that implement RowPreviewer.
type RowPreview struct {
	Index   int            `json:"index"`
	Data    map[string]any `json:"data"`
	Op      string         `json:"op,omitempty"`
	Changes []FieldChange  `json:"changes,omitempty"`
}

// FieldChange is one field an import row would change.
type FieldChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// RowDiff is the dry-run result of one import row. A non-empty Error marks
// the row invalid.
type RowDiff struct {
	Op      string
	Changes []FieldChange
	Error   string
}

// RowPreviewer is implemented by adapters that can dry-run an import row
// (validate it and list what it would create or change) without writing.
type RowPreviewer interface {
	PreviewRow(ctx context.Context, row map[string]any, defaults map[string]any) (RowDiff, error)
}

type RowError struct {
	Index int    `json:"index"`
	Field string `json:"field,omitempty"`
	Error string `json:"error"`
}

// Letterhead branding for PDF/XLSX headers.
type Letterhead struct {
	CompanyName  string
	Tagline      string
	PrimaryColor string
	Address      string
	Phone        string
	Email        string
	Website      string
	FooterText   string
	PaperSize    string
	LogoBytes    []byte
	LogoMIME     string
}

// SampleProvider is implemented by adapters that ship their own import
// sample rows (tenant resources living outside the adapters package).
type SampleProvider interface {
	SampleRows() []map[string]any
}

// ResourceAdapter connects ioengine to a list resource. Adapters of tenant
// resources ("tenant.*") read the organization with orgctx.OrganizationID:
// the export / import use cases set it from the request scope or the job row.
type ResourceAdapter interface {
	Resource() string
	ExportColumns() []Column
	Export(ctx context.Context, query ExportQuery, locale i18n.Locale) (Dataset, error)
	ImportSchema() []ImportField
	ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (RowResult, error)
	RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error
}

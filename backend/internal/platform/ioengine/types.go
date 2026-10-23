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

// PreviewSummary aggregates a dry-run.
type PreviewSummary struct {
	Total   int          `json:"total"`
	Valid   int          `json:"valid"`
	Invalid int          `json:"invalid"`
	Rows    []RowPreview `json:"rows"`
	Errors  []RowError   `json:"errors"`
}

type RowPreview struct {
	Index int            `json:"index"`
	Data  map[string]any `json:"data"`
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

// ResourceAdapter connects ioengine to a list resource.
type ResourceAdapter interface {
	Resource() string
	ExportColumns() []Column
	Export(ctx context.Context, query ExportQuery, locale i18n.Locale) (Dataset, error)
	ImportSchema() []ImportField
	ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (RowResult, error)
	RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error
}

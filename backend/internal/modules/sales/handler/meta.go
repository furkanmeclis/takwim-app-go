package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// LinesMeta is the table contract of GET /v1/tenant/sales/lines. Sale
// lines are written by the reservation and stock flows and are
// append-only: no create, update or delete (reversed or corrected).
func LinesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.LinesSlug,
		DefaultSort:   "-sale_date",
		DefaultFields: []string{"sale_date", "description", "item_type", "customer", "staff", "net_amount", "kind"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"description", "customer.name", "staff.name"},
		SortableFields:   []string{"sale_date", "net_amount", "kind", "created_at"},
		FilterableFields: []string{"branch", "kind", "item_type", "reservation", "customer", "staff", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "sale_date", LabelKey: "sales.columns.sale_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "description", LabelKey: "sales.columns.description", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "item_type", LabelKey: "sales.columns.item_type", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "customer", LabelKey: "sales.columns.customer", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "staff", LabelKey: "sales.columns.staff", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "branch", LabelKey: "sales.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "quantity", LabelKey: "sales.columns.quantity", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "unit_price", LabelKey: "sales.columns.unit_price", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "discount_amount", LabelKey: "sales.columns.discount_amount", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "net_amount", LabelKey: "sales.columns.net_amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "sales.columns.kind", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "reason", LabelKey: "sales.columns.reason", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "created_at", LabelKey: "sales.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "sales.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "sales.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "sale_line_kind"},
			{Key: "item_type", LabelKey: "sales.columns.item_type", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "sale_line_item_type"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// EntriesMeta is the table contract of GET /v1/tenant/sales/entries. The
// ledger is append-only: entries are posted by sales, payments and refunds
// and reversed, never edited.
func EntriesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.EntriesSlug,
		DefaultSort:   "-entry_date",
		DefaultFields: []string{"entry_date", "kind", "customer", "direction", "amount", "description"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"description", "customer.name"},
		SortableFields:   []string{"entry_date", "amount", "kind", "created_at"},
		FilterableFields: []string{"branch", "kind", "reservation", "customer", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "entry_date", LabelKey: "sales.entries.columns.entry_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "sales.entries.columns.kind", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "customer", LabelKey: "sales.entries.columns.customer", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "direction", LabelKey: "sales.entries.columns.direction", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: true},
			{Key: "amount", LabelKey: "sales.entries.columns.amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "method", LabelKey: "sales.entries.columns.method", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: false},
			{Key: "description", LabelKey: "sales.entries.columns.description", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "branch", LabelKey: "sales.entries.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "created_at", LabelKey: "sales.entries.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "sales.entries.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "sales.entries.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "ledger_entry_kind"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

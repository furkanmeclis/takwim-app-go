package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// EntriesMeta is the table contract of GET /v1/tenant/finance/entries.
// Records are append-only: no delete (reversed instead), no import, no
// bulk; the list exports to CSV / XLSX / PDF / JSON through the io engine.
func EntriesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.EntriesSlug,
		DefaultSort:   "-entry_date",
		DefaultFields: []string{"entry_date", "kind", "category", "amount", "branch", "description"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: false,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: false,
		},
		SearchableFields: []string{"description", "category.name", "staff.name"},
		SortableFields:   []string{"entry_date", "amount", "created_at"},
		FilterableFields: []string{"source", "kind", "category", "branch", "staff", "account", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "entry_date", LabelKey: "finance.columns.entry_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "finance.columns.kind", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "source", LabelKey: "finance.columns.source", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "category", LabelKey: "finance.columns.category", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "amount", LabelKey: "finance.columns.amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "branch", LabelKey: "finance.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "staff", LabelKey: "finance.columns.staff", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "account", LabelKey: "finance.columns.account", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "description", LabelKey: "finance.columns.description", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "created_at", LabelKey: "finance.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "finance.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "finance.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "finance_kind"},
			{Key: "source", LabelKey: "finance.columns.source", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "finance_source"},
			{Key: "category", LabelKey: "finance.columns.category", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "finance.facets.category"},
			{Key: "branch", LabelKey: "finance.columns.branch", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "finance.facets.branch"},
			{Key: "staff", LabelKey: "finance.columns.staff", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "finance.facets.staff"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

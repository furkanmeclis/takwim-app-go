package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/services. Capabilities
// match the live routes: export/import/bulk are mounted by routes.go.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultSort:   "",
		DefaultFields: []string{"uuid", "name", "category", "duration_minutes", "price", "payment_mode", "show_online", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"name", "description", "category.name"},
		SortableFields:   []string{"name", "duration", "price", "sort_order", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"category", "branch", "payment_mode", "is_active", "show_online"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "services.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "category", LabelKey: "services.columns.category", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "duration_minutes", LabelKey: "services.columns.duration", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "price", LabelKey: "services.columns.price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "payment_mode", LabelKey: "services.columns.payment_mode", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "services.columns.branches", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "staff_mode", LabelKey: "services.columns.staff_mode", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: false},
			{Key: "show_online", LabelKey: "services.columns.show_online", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "is_active", LabelKey: "services.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "sort_order", LabelKey: "services.columns.sort_order", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "created_at", LabelKey: "services.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "services.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "category", LabelKey: "services.columns.category", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "services.facets.category"},
			{Key: "branch", LabelKey: "services.columns.branches", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "services.facets.branch"},
			{Key: "payment_mode", LabelKey: "services.columns.payment_mode", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "services.facets.payment_mode"},
			{Key: "is_active", LabelKey: "services.columns.is_active", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "show_online", LabelKey: "services.columns.show_online", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

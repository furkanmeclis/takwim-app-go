package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/resources. Capabilities
// match the live routes: export/import/bulk are mounted by routes.go.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultSort:   "",
		DefaultFields: []string{"uuid", "name", "kind", "branch", "zone", "capacity", "show_online", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"name", "zone", "description", "branch.name"},
		SortableFields:   []string{"name", "kind", "zone", "capacity", "sort_order", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"branch", "kind", "zone", "is_active", "show_online"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "resources.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "resources.columns.kind", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "resources.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "zone", LabelKey: "resources.columns.zone", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "capacity", LabelKey: "resources.columns.capacity", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "combinable", LabelKey: "resources.columns.combinable", Type: resourcemeta.ColumnTypeBoolean, DefaultVisible: false},
			{Key: "show_online", LabelKey: "resources.columns.show_online", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "is_active", LabelKey: "resources.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "sort_order", LabelKey: "resources.columns.sort_order", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "created_at", LabelKey: "resources.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "resources.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "branch", LabelKey: "resources.columns.branch", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "resources.facets.branch"},
			{Key: "kind", LabelKey: "resources.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "resources.facets.kind"},
			{Key: "zone", LabelKey: "resources.columns.zone", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "resources.facets.zone"},
			{Key: "is_active", LabelKey: "resources.columns.is_active", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "show_online", LabelKey: "resources.columns.show_online", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

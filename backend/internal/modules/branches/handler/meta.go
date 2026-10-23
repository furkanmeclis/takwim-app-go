package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/branches. Capabilities match
// the live routes: export/import/bulk are mounted by routes.go.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Resource,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "code", "name", "city", "district", "phone", "is_active", "service_points_count"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"code", "name", "city", "district", "phone", "email"},
		SortableFields:   []string{"code", "name", "city", "prep_minutes", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"is_active", "city"},
		Columns: []resourcemeta.Column{
			{Key: "code", LabelKey: "branches.columns.code", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "name", LabelKey: "branches.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "city", LabelKey: "branches.columns.city", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "district", LabelKey: "branches.columns.district", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "phone", LabelKey: "branches.columns.phone", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "email", LabelKey: "branches.columns.email", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "prep_minutes", LabelKey: "branches.columns.prep_minutes", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "service_points_count", LabelKey: "branches.columns.service_points_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "is_active", LabelKey: "branches.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "branches.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "branches.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "is_active", LabelKey: "branches.columns.is_active", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "city", LabelKey: "branches.columns.city", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "branches.facets.city"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

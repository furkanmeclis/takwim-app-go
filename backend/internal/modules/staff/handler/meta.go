package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/staff. Capabilities match
// the live routes: export/import/bulk are mounted by routes.go.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Resource,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "name", "title", "branches", "phone", "account", "show_online", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"name", "title", "phone", "email", "account.email"},
		SortableFields:   []string{"name", "title", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"branch", "is_active", "show_online", "has_account"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "staff.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "title", LabelKey: "staff.columns.title", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "branches", LabelKey: "staff.columns.branches", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "phone", LabelKey: "staff.columns.phone", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "email", LabelKey: "staff.columns.email", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "account", LabelKey: "staff.columns.account", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "show_online", LabelKey: "staff.columns.show_online", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "is_active", LabelKey: "staff.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "staff.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "staff.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "branch", LabelKey: "staff.columns.branches", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "staff.facets.branch"},
			{Key: "is_active", LabelKey: "staff.columns.is_active", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "show_online", LabelKey: "staff.columns.show_online", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "has_account", LabelKey: "staff.columns.account", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

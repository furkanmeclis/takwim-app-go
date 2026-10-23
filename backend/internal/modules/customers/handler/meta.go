package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/customers. Capabilities
// match the live routes: export/import/bulk are mounted by routes.go.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultSort:   "",
		DefaultFields: []string{"uuid", "name", "phone", "email", "tags", "verified", "source", "created_at"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"first_name", "last_name", "email", "phone"},
		SortableFields:   []string{"name", "phone", "email", "source", "created_at", "updated_at"},
		FilterableFields: []string{"tag", "source", "sms_consent", "email_consent", "verified"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "customers.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "phone", LabelKey: "customers.columns.phone", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "email", LabelKey: "customers.columns.email", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "tags", LabelKey: "customers.columns.tags", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "verified", LabelKey: "customers.columns.verified", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "source", LabelKey: "customers.columns.source", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "sms_consent", LabelKey: "customers.columns.sms_consent", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "email_consent", LabelKey: "customers.columns.email_consent", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "created_at", LabelKey: "customers.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "uuid", LabelKey: "customers.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "tag", LabelKey: "customers.columns.tags", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "customers.facets.tag"},
			{Key: "source", LabelKey: "customers.columns.source", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "customers.facets.source"},
			{Key: "verified", LabelKey: "customers.columns.verified", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "sms_consent", LabelKey: "customers.columns.sms_consent", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "email_consent", LabelKey: "customers.columns.email_consent", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

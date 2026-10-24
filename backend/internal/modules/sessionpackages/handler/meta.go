package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// PackagesMeta is the table contract of GET /v1/tenant/session-packages.
func PackagesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.PackagesSlug,
		DefaultSort:   "name",
		DefaultFields: []string{"name", "service", "total_sessions", "price", "session_value", "validity_days", "branches", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Create: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true, Bulk: true,
		},
		SearchableFields: []string{"name", "service.name"},
		SortableFields:   []string{"name", "price", "total_sessions", "created_at"},
		FilterableFields: []string{"is_active", "service", "branch"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "session_packages.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "service", LabelKey: "session_packages.columns.service", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "total_sessions", LabelKey: "session_packages.columns.total_sessions", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "price", LabelKey: "session_packages.columns.price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "session_value", LabelKey: "session_packages.columns.session_value", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "validity_days", LabelKey: "session_packages.columns.validity_days", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "branches", LabelKey: "session_packages.columns.branches", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "is_active", LabelKey: "session_packages.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "sold_count", LabelKey: "session_packages.columns.sold_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "created_at", LabelKey: "session_packages.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "session_packages.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "is_active", LabelKey: "session_packages.columns.is_active", Variant: resourcemeta.FilterVariantFaceted},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

// CustomerPackagesMeta is the table contract of GET /v1/tenant/customer-packages.
// A sold package is not edited: it is used session by session or refunded.
func CustomerPackagesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.CustomerPackagesSlug,
		DefaultSort:   "-sold_on",
		DefaultFields: []string{"customer", "name", "sessions", "remaining_amount", "price", "sold_on", "expires_on", "state"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Create: true, Search: true, Filter: true, Sort: true, Export: true,
		},
		SearchableFields: []string{"name", "service_name", "customer.name", "customer.phone"},
		SortableFields:   []string{"sold_on", "expires_on", "price", "created_at"},
		FilterableFields: []string{"state", "package", "branch", "customer", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "customer", LabelKey: "session_packages.customer.columns.customer", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "name", LabelKey: "session_packages.customer.columns.name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "package", LabelKey: "session_packages.customer.columns.package", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "sessions", LabelKey: "session_packages.customer.columns.sessions", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "remaining_amount", LabelKey: "session_packages.customer.columns.remaining_amount", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "price", LabelKey: "session_packages.customer.columns.price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "sold_on", LabelKey: "session_packages.customer.columns.sold_on", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "expires_on", LabelKey: "session_packages.customer.columns.expires_on", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "state", LabelKey: "session_packages.customer.columns.state", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "session_packages.customer.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "created_at", LabelKey: "session_packages.customer.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "session_packages.customer.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "state", LabelKey: "session_packages.customer.columns.state", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "customer_package_state"},
			{Key: "package", LabelKey: "session_packages.customer.columns.package", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "branch", LabelKey: "session_packages.customer.columns.branch", Variant: resourcemeta.FilterVariantFaceted},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

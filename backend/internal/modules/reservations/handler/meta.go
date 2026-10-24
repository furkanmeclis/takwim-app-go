package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// BulkActions are the bulk transitions of the list (TEC-963). Cancel takes
// params["reason"] (default "cancelled"); every action takes
// params["versions"] ("uuid:version,…", the versions the caller read), and a
// record changed since is skipped as stale. None can be rolled back.
func BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "accept", LabelKey: "bulk.actions.reservations.accept",
			Permission: rbac.PermTenantReservationsBulkAccept,
			ConfirmKey: "bulk.confirm.reservations.accept",
		},
		{
			ID: "reject", LabelKey: "bulk.actions.reservations.reject",
			Permission: rbac.PermTenantReservationsBulkReject, Destructive: true,
			ConfirmKey: "bulk.confirm.reservations.reject",
		},
		{
			ID: "cancel", LabelKey: "bulk.actions.reservations.cancel",
			Permission: rbac.PermTenantReservationsBulkCancel, Destructive: true,
			ConfirmKey: "bulk.confirm.reservations.cancel",
		},
	}
}

// Meta is the table contract of GET /v1/tenant/reservations. Export and
// bulk are mounted by RegisterEngineRoutes (no import: a booking always goes
// through the capacity engine).
func Meta() resourcemeta.ResourceMeta {
	enum := func(key string) resourcemeta.Column {
		return resourcemeta.Column{
			Key: key, LabelKey: "reservations.columns." + key, Type: resourcemeta.ColumnTypeEnum,
			Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true,
		}
	}
	status := enum("status")
	status.Sortable = true
	source := enum("source")
	source.DefaultVisible = false
	// Customer confirmation (TEC-921): shown when the add-on is on.
	confirmation := enum("confirmation_status")
	confirmation.DefaultVisible = false
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultSort:   "-starts_at",
		DefaultFields: []string{"number", "starts_at", "customer", "services", "staff", "branch", "status", "approval_status", "payment_status", "total_price"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
			Export: true, Bulk: true,
		},
		SearchableFields: []string{"number", "customer", "phone"},
		SortableFields:   []string{"starts_at", "number", "customer", "status", "total_price", "created_at"},
		FilterableFields: []string{"branch", "staff", "status", "approval_status", "confirmation_status", "payment_status", "source", "starts_from", "starts_to", "customer_uuid"},
		Columns: []resourcemeta.Column{
			{Key: "number", LabelKey: "reservations.columns.number", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "starts_at", LabelKey: "reservations.columns.starts_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "customer", LabelKey: "reservations.columns.customer", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "services", LabelKey: "reservations.columns.services", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "staff", LabelKey: "reservations.columns.staff", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "reservations.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			status, enum("approval_status"), confirmation, enum("payment_status"), source,
			{Key: "total_price", LabelKey: "reservations.columns.total_price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "reservations.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "reservations.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "branch", LabelKey: "reservations.columns.branch", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.facets.branch"},
			{Key: "staff", LabelKey: "reservations.columns.staff", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.facets.staff"},
			{Key: "status", LabelKey: "reservations.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.status"},
			{Key: "approval_status", LabelKey: "reservations.columns.approval_status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.approval_status"},
			{Key: "confirmation_status", LabelKey: "reservations.columns.confirmation_status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.confirmation_status"},
			{Key: "payment_status", LabelKey: "reservations.columns.payment_status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.payment_status"},
			{Key: "source", LabelKey: "reservations.columns.source", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "reservations.source"},
		},
		Includes:    []string{},
		BulkActions: BulkActions(),
	}
}

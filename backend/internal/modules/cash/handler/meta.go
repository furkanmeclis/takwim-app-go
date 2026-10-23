package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// AccountsMeta is the table contract of GET /v1/tenant/cash/accounts.
// Capabilities match the live routes (export and bulk; no import, no
// delete: an account is archived, never removed).
func AccountsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.AccountsSlug,
		DefaultFields: []string{"uuid", "name", "kind", "branch", "balance", "status"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: false,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: true,
		},
		SearchableFields: []string{"name", "bank_name", "iban", "branch.name"},
		SortableFields:   []string{"name", "kind", "balance", "status", "sort_order", "created_at"},
		FilterableFields: []string{"branch", "kind", "status"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "cash.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "cash.columns.kind", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "cash.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "balance", LabelKey: "cash.columns.balance", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "last_closed_date", LabelKey: "cash.columns.last_closed_date", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "status", LabelKey: "cash.columns.status", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "bank_name", LabelKey: "cash.columns.bank_name", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "opening_balance", LabelKey: "cash.columns.opening_balance", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "created_at", LabelKey: "cash.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "cash.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "branch", LabelKey: "cash.columns.branch", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash.facets.branch"},
			{Key: "kind", LabelKey: "cash.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_account_kind"},
			{Key: "status", LabelKey: "cash.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_account_status"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

// MovementsMeta is the table contract of GET /v1/tenant/cash/movements.
// Movements are append-only: no create (posted on an account or as a
// transfer), no update, no delete (reversed instead).
func MovementsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.MovementsSlug,
		DefaultSort:   "-movement_date",
		DefaultFields: []string{"movement_date", "account", "kind", "direction", "amount", "description"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true, Export: true,
		},
		SearchableFields: []string{"description", "account.name"},
		SortableFields:   []string{"movement_date", "amount", "kind", "created_at"},
		FilterableFields: []string{"account", "kind", "direction", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "movement_date", LabelKey: "cash.movements.columns.movement_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "account", LabelKey: "cash.movements.columns.account", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "kind", LabelKey: "cash.movements.columns.kind", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "direction", LabelKey: "cash.movements.columns.direction", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "amount", LabelKey: "cash.movements.columns.amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "description", LabelKey: "cash.movements.columns.description", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "created_by", LabelKey: "cash.movements.columns.created_by", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "created_at", LabelKey: "cash.movements.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "cash.movements.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "account", LabelKey: "cash.movements.columns.account", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash.movements.facets.account"},
			{Key: "kind", LabelKey: "cash.movements.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_movement_kind"},
			{Key: "direction", LabelKey: "cash.movements.columns.direction", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_movement_direction"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// LinesMeta is the table contract of GET /v1/tenant/staff-shares/lines.
// Shares are written with their sale lines and are append-only: no
// create, update or delete (a reversed sale line reverses its share).
func LinesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.LinesSlug,
		DefaultSort:   "-share_date",
		DefaultFields: []string{"share_date", "staff", "description", "base_amount", "rate", "share_amount", "status"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"description", "staff.name", "customer.name"},
		SortableFields:   []string{"share_date", "share_amount", "base_amount", "created_at"},
		FilterableFields: []string{"branch", "staff", "kind", "item_type", "status", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "share_date", LabelKey: "staff_shares.columns.share_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "staff", LabelKey: "staff_shares.columns.staff", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "description", LabelKey: "staff_shares.columns.description", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "item_type", LabelKey: "staff_shares.columns.item_type", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "base_amount", LabelKey: "staff_shares.columns.base_amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "rate", LabelKey: "staff_shares.columns.rate", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "share_amount", LabelKey: "staff_shares.columns.share_amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "status", LabelKey: "staff_shares.columns.status", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "kind", LabelKey: "staff_shares.columns.kind", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "branch", LabelKey: "staff_shares.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "customer", LabelKey: "staff_shares.columns.customer", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "created_at", LabelKey: "staff_shares.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "staff_shares.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "staff_shares.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "staff_share_status"},
			{Key: "item_type", LabelKey: "staff_shares.columns.item_type", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "staff_share_item_type"},
			{Key: "kind", LabelKey: "staff_shares.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "staff_share_kind"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// PayoutsMeta is the table contract of GET /v1/tenant/staff-shares/payouts.
// Payouts are append-only: a wrong one is reversed, never edited.
func PayoutsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.PayoutsSlug,
		DefaultSort:   "-payout_date",
		DefaultFields: []string{"payout_date", "staff", "amount", "cash_account", "kind", "note"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Create: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"note", "staff.name"},
		SortableFields:   []string{"payout_date", "amount", "created_at"},
		FilterableFields: []string{"branch", "staff", "kind", "date_from", "date_to"},
		Columns: []resourcemeta.Column{
			{Key: "payout_date", LabelKey: "staff_shares.payouts.columns.payout_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "staff", LabelKey: "staff_shares.payouts.columns.staff", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "amount", LabelKey: "staff_shares.payouts.columns.amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "cash_account", LabelKey: "staff_shares.payouts.columns.cash_account", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "kind", LabelKey: "staff_shares.payouts.columns.kind", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "note", LabelKey: "staff_shares.payouts.columns.note", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "branch", LabelKey: "staff_shares.payouts.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "created_at", LabelKey: "staff_shares.payouts.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "staff_shares.payouts.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "staff_shares.payouts.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "staff_payout_kind"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

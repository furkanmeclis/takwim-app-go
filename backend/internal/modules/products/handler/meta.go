package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/products. Capabilities
// match the live routes: export/import/bulk are mounted by routes.go.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultSort:   "",
		DefaultFields: []string{"uuid", "name", "sku", "price", "stock", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"name", "sku", "barcode", "variants.name", "variants.sku", "variants.barcode"},
		SortableFields:   []string{"name", "sku", "price", "cost", "stock", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"branch", "stock_status", "is_active", "show_online"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "products.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "sku", LabelKey: "products.columns.sku", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "price", LabelKey: "products.columns.price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "cost", LabelKey: "products.columns.cost", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "stock", LabelKey: "products.columns.stock", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "stock_status", LabelKey: "products.columns.stock_status", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "products.columns.branches", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "variants_count", LabelKey: "products.columns.variants", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "show_online", LabelKey: "products.columns.show_online", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "is_active", LabelKey: "products.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "products.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "products.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "branch", LabelKey: "products.columns.branches", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "products.facets.branch"},
			{Key: "stock_status", LabelKey: "products.columns.stock_status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "products.stock_status"},
			{Key: "is_active", LabelKey: "products.columns.is_active", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "show_online", LabelKey: "products.columns.show_online", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

// MovementMeta is the table contract of GET /v1/tenant/stock-movements
// (the append-only ledger: no update, delete or bulk; export and the
// count import are mounted by routes.go).
func MovementMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.MovementsSlug,
		DefaultSort:   "-occurred_at",
		DefaultFields: []string{"uuid", "occurred_at", "kind", "product", "branch", "quantity", "balance_after"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Search: true, Filter: true, Sort: true, Export: true, Import: true,
		},
		SearchableFields: []string{"product.name", "product.sku", "variant.sku", "reason", "note"},
		SortableFields:   []string{"occurred_at", "quantity", "kind"},
		FilterableFields: []string{"kind", "branch", "product", "staff", "from", "to"},
		Columns: []resourcemeta.Column{
			{Key: "occurred_at", LabelKey: "products.movements.columns.occurred_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "products.movements.columns.kind", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "product", LabelKey: "products.movements.columns.product", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "branch", LabelKey: "products.movements.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "quantity", LabelKey: "products.movements.columns.quantity", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "balance_after", LabelKey: "products.movements.columns.balance_after", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "staff", LabelKey: "products.movements.columns.staff", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "reason", LabelKey: "products.movements.columns.reason", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "created_by", LabelKey: "products.movements.columns.created_by", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "uuid", LabelKey: "products.movements.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "products.movements.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "stock.facets.kind"},
			{Key: "branch", LabelKey: "products.movements.columns.branch", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "stock.facets.branch"},
			{Key: "staff", LabelKey: "products.movements.columns.staff", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "stock.facets.staff"},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

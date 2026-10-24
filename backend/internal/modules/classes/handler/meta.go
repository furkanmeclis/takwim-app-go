package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// ScheduleMeta is the table contract of GET /v1/tenant/classes.
func ScheduleMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultSort:   "weekday",
		DefaultFields: []string{"weekday", "start_time", "service", "staff", "resource", "seats", "validity", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Create: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"service.name", "staff.name", "resource.name", "note"},
		SortableFields:   []string{"weekday", "service", "created_at"},
		FilterableFields: []string{"branch", "service", "staff", "weekday", "is_active"},
		Columns: []resourcemeta.Column{
			{Key: "weekday", LabelKey: "classes.columns.weekday", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "start_time", LabelKey: "classes.columns.start_time", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "service", LabelKey: "classes.columns.service", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch", LabelKey: "classes.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "staff", LabelKey: "classes.columns.staff", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "resource", LabelKey: "classes.columns.resource", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "seats", LabelKey: "classes.columns.seats", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "validity", LabelKey: "classes.columns.validity", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "is_active", LabelKey: "classes.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "note", LabelKey: "classes.columns.note", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "created_at", LabelKey: "classes.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "classes.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "weekday", LabelKey: "classes.columns.weekday", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "service", LabelKey: "classes.columns.service", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "branch", LabelKey: "classes.columns.branch", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "staff", LabelKey: "classes.columns.staff", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "is_active", LabelKey: "classes.columns.is_active", Variant: resourcemeta.FilterVariantFaceted},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// Package adapters connects the platform organizations list to the shared
// engines: ioengine (export) and bulkengine (activate / suspend). It is a
// platform resource: the context never carries an organization.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// Resource is the engine slug of the platform organizations list.
const Resource = orghandler.ResourcePlatformOrganizations

// ExportQueryKeys are the list filters an export request may carry.
var ExportQueryKeys = []string{"status", "access", "window"}

// MaxTargets caps export rows and the "all matching" bulk scope.
const MaxTargets = 10000

var errNoImport = errors.New("organizations: import is not supported")

func filterFromQuery(query map[string]string) (orgmodel.ListFilter, error) {
	f, field, err := orgmodel.OrganizationFilterFromValues(func(k string) string { return query[k] })
	if err != nil {
		return f, fmt.Errorf("%s: %w", field, err)
	}
	f.SortField, f.SortDesc = "created_at", true
	if sort := strings.TrimSpace(query["sort"]); sort != "" {
		f.SortDesc = strings.HasPrefix(sort, "-")
		f.SortField = strings.TrimPrefix(sort, "-")
		if _, ok := orghandler.OrganizationSortColumns[f.SortField]; !ok {
			f.SortField, f.SortDesc = "created_at", true
		}
	}
	f.Limit, f.Offset = MaxTargets, 0
	return f, nil
}

// IO exports the platform organizations list (export only).
type IO struct {
	svc *orgusecase.Service
}

// NewIO builds the export adapter.
func NewIO(svc *orgusecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "organizations." + key, Type: t}
	}
	s, dt := ioengine.ColumnTypeString, ioengine.ColumnTypeDatetime
	return []ioengine.Column{
		col("name", s), col("slug", s), col("owner_email", s), col("owner_name", s),
		col("member_count", s), col("branch_count", s),
		col("status", ioengine.ColumnTypeEnum), col("access_state", ioengine.ColumnTypeEnum),
		col("access_starts_at", dt), col("access_ends_at", dt),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", dt),
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	if _, ok := orgctx.OrganizationID(ctx); ok {
		return ioengine.Dataset{}, errors.New("organizations: platform resource exported inside an organization")
	}
	f, err := filterFromQuery(query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, _, err := a.svc.ListOrganizations(ctx, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, o := range rows {
		row := map[string]any{
			"uuid": o.UUID.String(), "name": o.Name, "slug": o.Slug, "owner_email": "", "owner_name": "",
			"member_count": fmt.Sprint(o.MemberCount), "branch_count": fmt.Sprint(o.BranchCount),
			"status": o.Status, "access_state": o.AccessState,
			"access_starts_at": o.AccessStartsAt, "access_ends_at": "", "created_at": o.CreatedAt,
		}
		if o.Owner != nil {
			row["owner_email"] = o.Owner.Email
			row["owner_name"] = strings.TrimSpace(o.Owner.Name + " " + o.Owner.Surname)
		}
		if o.AccessEndsAt != nil {
			row["access_ends_at"] = *o.AccessEndsAt
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }

// Bulk activates / suspends organizations (reversible). Status changes go
// through the use case so the organization cache is invalidated; the bulk
// job publishes one list refresh when it finishes.
type Bulk struct {
	svc *orgusecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *orgusecase.Service) *Bulk { return &Bulk{svc: svc} }

func (a *Bulk) Resource() string { return Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef { return orghandler.OrganizationBulkActions() }

func (a *Bulk) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxTargets)
		}
		return out, nil
	case "query":
		if a.svc == nil {
			return nil, errors.New("organizations bulk: no service")
		}
		f, err := filterFromQuery(target.Query)
		if err != nil {
			return nil, err
		}
		rows, _, err := a.svc.ListOrganizations(ctx, f)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(rows))
		for _, o := range rows {
			out = append(out, o.UUID.String())
		}
		return out, nil
	default:
		return nil, errors.New("invalid target scope")
	}
}

func statusFor(action string) (string, bool) {
	switch action {
	case "activate":
		return orgctx.StatusActive, true
	case "suspend":
		return orgctx.StatusSuspended, true
	default:
		return "", false
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	status, ok := statusFor(action)
	if !ok {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	if a.svc == nil {
		return bulkengine.BulkItemResult{}, errors.New("organizations bulk: no service")
	}
	current, err := a.svc.GetOrganization(ctx, id)
	if err != nil {
		if errors.Is(err, orgusecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	previous := map[string]any{"status": current.Status}
	if current.Status != status {
		if _, err := a.svc.SetStatus(ctx, id, status); err != nil {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "organization", OK: true, Op: "update", Previous: previous,
	}, nil
}

func (a *Bulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	status, _ := previous["status"].(string)
	if status != orgctx.StatusActive && status != orgctx.StatusSuspended {
		return nil
	}
	_, err = a.svc.SetStatus(ctx, id, status)
	return err
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
)

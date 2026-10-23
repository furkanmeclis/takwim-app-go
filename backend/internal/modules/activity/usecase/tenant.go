package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// TenantActor is who did it (null for system events).
type TenantActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// TenantBranch is the branch an event happened in.
type TenantBranch struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// TenantEvent is one row of the organization's activity history. Payload
// carries the action details; "changes" in it holds the before / after of
// the changed fields ({"field": {"before": …, "after": …}}).
type TenantEvent struct {
	UUID         uuid.UUID       `json:"uuid"`
	Action       string          `json:"action"`
	Resource     string          `json:"resource"`
	ResourceUUID *uuid.UUID      `json:"resource_uuid"`
	Payload      json.RawMessage `json:"payload"`
	Actor        *TenantActor    `json:"actor"`
	Branch       *TenantBranch   `json:"branch"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TenantFilter filters GET /v1/tenant/activity.
type TenantFilter struct {
	Limit, Offset int32
	Q             string
	Resources     []string
	Actions       []string
	ResourceUUID  *uuid.UUID
	ActorUUID     *uuid.UUID
	From, To      *time.Time
	// SortDesc lists newest first (default).
	SortDesc bool
}

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// TenantFacets are the resources and actions present in the caller's scope.
type TenantFacets struct {
	Resources []Facet `json:"resources"`
	Actions   []Facet `json:"actions"`
}

// TenantList returns one page of the organization's activity. The caller's
// branch scope (narrowed by X-Branch) applies: a member limited to some
// branches, or a picked branch, sees only events of those branches.
func (s *Service) TenantList(ctx context.Context, orgID int64, f TenantFilter) ([]TenantEvent, int64, error) {
	branchIDs := orgctx.BranchScope(ctx)
	q := pgtype.Text{String: f.Q, Valid: f.Q != ""}
	rows, err := s.q.ListTenantActivity(ctx, db.ListTenantActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, BranchIds: branchIDs, Resources: f.Resources, Actions: f.Actions,
		ResourceUuid: uuidArg(f.ResourceUUID), ActorUuid: uuidArg(f.ActorUUID),
		CreatedFrom: timeArg(f.From), CreatedTo: timeArg(f.To), Q: q,
		SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountTenantActivity(ctx, db.CountTenantActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, BranchIds: branchIDs, Resources: f.Resources, Actions: f.Actions,
		ResourceUuid: uuidArg(f.ResourceUUID), ActorUuid: uuidArg(f.ActorUUID),
		CreatedFrom: timeArg(f.From), CreatedTo: timeArg(f.To), Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]TenantEvent, 0, len(rows))
	for _, row := range rows {
		e := TenantEvent{
			UUID: row.Uuid, Action: row.Action, Resource: row.Resource,
			Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if len(e.Payload) == 0 {
			e.Payload = json.RawMessage("{}")
		}
		if row.ResourceUuid.Valid {
			id := uuid.UUID(row.ResourceUuid.Bytes)
			e.ResourceUUID = &id
		}
		if row.ActorUuid.Valid {
			e.Actor = &TenantActor{
				UUID: row.ActorUuid.Bytes, Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		if row.BranchUuid.Valid {
			e.Branch = &TenantBranch{UUID: row.BranchUuid.Bytes, Name: row.BranchName.String}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// TenantFacets returns the resources and actions in the caller's scope.
func (s *Service) TenantFacets(ctx context.Context, orgID int64) (TenantFacets, error) {
	rows, err := s.q.TenantActivityFacets(ctx, db.TenantActivityFacetsParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return TenantFacets{}, err
	}
	out := TenantFacets{Resources: []Facet{}, Actions: []Facet{}}
	byResource := map[string]int{}
	for _, row := range rows {
		if i, ok := byResource[row.Resource]; ok {
			out.Resources[i].Count += row.Count
		} else {
			byResource[row.Resource] = len(out.Resources)
			out.Resources = append(out.Resources, Facet{Value: row.Resource, Count: row.Count})
		}
		out.Actions = append(out.Actions, Facet{Value: row.Action, Count: row.Count})
	}
	return out, nil
}

func uuidArg(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func timeArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

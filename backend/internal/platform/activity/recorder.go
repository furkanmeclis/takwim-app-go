package activity

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recorder persists audit events.
type Recorder struct {
	q   *db.Queries
	log *slog.Logger
}

// NewRecorder creates an activity recorder.
func NewRecorder(q *db.Queries, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{q: q, log: log}
}

// Entry is one activity event: who (ActorID), what (Action on Resource /
// ResourceUUID), where (BranchUUID: the branch it happened in, nil for the
// whole business) and the before / after values of the changed fields
// (Changes, see Diff), stored in the payload under "changes".
type Entry struct {
	ActorID      *int64
	Action       string
	Resource     string
	ResourceUUID *uuid.UUID
	BranchUUID   *uuid.UUID
	Payload      map[string]any
	Changes      map[string]Change
	Request      *http.Request
}

// Record writes an activity event of the whole business (fail-soft).
func (r *Recorder) Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, rreq *http.Request) {
	r.RecordEntry(ctx, Entry{
		ActorID: actorID, Action: action, Resource: resource, ResourceUUID: resourceUUID,
		Payload: payload, Request: rreq,
	})
}

// RecordEntry writes an activity event (fail-soft).
func (r *Recorder) RecordEntry(ctx context.Context, e Entry) {
	if r == nil || r.q == nil {
		return
	}
	actorID, action, resource, resourceUUID, payload, rreq := e.ActorID, e.Action, e.Resource, e.ResourceUUID, e.Payload, e.Request
	if len(e.Changes) > 0 {
		merged := make(map[string]any, len(payload)+1)
		for k, v := range payload {
			merged[k] = v
		}
		merged["changes"] = e.Changes
		payload = merged
	}
	body, err := json.Marshal(payload)
	if err != nil || payload == nil {
		body = []byte("{}")
	}
	var ru pgtype.UUID
	if resourceUUID != nil {
		ru = pgtype.UUID{Bytes: *resourceUUID, Valid: true}
	}
	var ip *netip.Addr
	var ua pgtype.Text
	if rreq != nil {
		if host, _, err := net.SplitHostPort(rreq.RemoteAddr); err == nil {
			if parsed, err := netip.ParseAddr(host); err == nil {
				ip = &parsed
			}
		}
		if rreq.UserAgent() != "" {
			ua = pgtype.Text{String: rreq.UserAgent(), Valid: true}
		}
	}
	// Tenant requests and organization-scoped jobs stamp the organization so
	// the event shows up in that organization's activity only.
	var orgID pgtype.Int8
	if id, ok := orgctx.OrganizationID(ctx); ok {
		orgID = pgtype.Int8{Int64: id, Valid: true}
	}
	var branch pgtype.UUID
	if e.BranchUUID != nil && orgID.Valid {
		branch = pgtype.UUID{Bytes: *e.BranchUUID, Valid: true}
	}
	_, err = r.q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		OrganizationID: orgID,
		ActorUserID:    pgtypeInt8(actorID),
		Action:         action,
		Resource:       resource,
		ResourceUuid:   ru,
		Payload:        body,
		IpAddress:      ip,
		UserAgent:      ua,
		BranchUuid:     branch,
	})
	if err != nil {
		r.log.Warn("activity_record_failed", "action", action, "error", err)
	}
}

func pgtypeInt8(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// InboundInput is a provider notification (webhook, delivery report,
// incoming message). Key is the provider's own event id, so a notification
// delivered twice maps to one job.
type InboundInput struct {
	OrganizationID int64
	AccountID      int64
	Provider       string
	Kind           string
	Key            string
	// Payload holds references and non-secret facts only.
	Payload map[string]any
	RefType string
	RefUUID *uuid.UUID
}

// InboundResult tells the caller what happened to a notification.
type InboundResult struct {
	JobUUID    uuid.UUID
	Status     string
	Deliveries int32
	// Duplicate: the notification was already handled (or is being handled
	// right now); the handler was not called again.
	Duplicate bool
}

// InboundHandler applies a notification once. It returns the final status
// (StatusSucceeded, or StatusIgnored when no action is needed) and a short
// result summary; an error marks the job failed so the next delivery of the
// same notification runs the handler again.
type InboundHandler func(ctx context.Context) (status string, result map[string]any, err error)

// ErrInboundInput: the notification lacks its organization, provider, kind
// or key.
var ErrInboundInput = errors.New("integration notification needs organization, provider, kind and key")

// HandleInbound records a provider notification exactly once per (provider,
// key) and runs fn at most once successfully: a repeated delivery of a
// handled notification only counts the delivery, a failed one is retried
// by the next delivery. Concurrent deliveries are serialised by the claim
// (pending / failed → processing).
func (r *Runner) HandleInbound(ctx context.Context, in InboundInput, fn InboundHandler) (InboundResult, error) {
	if in.OrganizationID <= 0 || in.Provider == "" || in.Kind == "" || in.Key == "" || fn == nil {
		return InboundResult{}, ErrInboundInput
	}
	var ref pgtype.UUID
	if in.RefUUID != nil {
		ref = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
	}
	org := orgParam(in.OrganizationID)
	row, err := r.q.UpsertIntegrationJob(ctx, db.UpsertIntegrationJobParams{
		OrganizationID: org, IntegrationAccountID: pgtype.Int8{Int64: in.AccountID, Valid: in.AccountID > 0},
		Direction: "inbound", Provider: in.Provider, Kind: in.Kind, IdempotencyKey: trim(in.Key, 200),
		Payload: jsonOf(in.Payload), RefType: trim(in.RefType, 24), RefUuid: ref,
	})
	if err != nil {
		return InboundResult{}, err
	}
	out := InboundResult{JobUUID: row.Uuid, Status: row.Status, Deliveries: row.Deliveries}
	claimed, err := r.q.ClaimIntegrationJob(ctx, db.ClaimIntegrationJobParams{OrganizationID: org, ID: row.ID})
	if err != nil {
		return out, err
	}
	if claimed == 0 {
		out.Duplicate = true
		return out, nil
	}
	status, result, hErr := runInbound(ctx, fn)
	lastError := ""
	if hErr != nil {
		status, lastError = StatusFailed, trim(hErr.Error(), 1000)
	} else if status != StatusIgnored {
		status = StatusSucceeded
	}
	if err := r.q.FinishIntegrationJob(ctx, db.FinishIntegrationJobParams{
		Status: status, LastError: lastError, Result: jsonOf(result), OrganizationID: org, ID: row.ID,
	}); err != nil {
		return out, err
	}
	out.Status = status
	r.publish(ctx, in.OrganizationID, row.Uuid)
	return out, hErr
}

// runInbound turns a handler panic into a failure, so the job never stays
// claimed (processing) and the next delivery can run it again.
func runInbound(ctx context.Context, fn InboundHandler) (status string, result map[string]any, err error) {
	defer func() {
		if p := recover(); p != nil {
			status, result, err = StatusFailed, nil, fmt.Errorf("handler panic: %v", p)
		}
	}()
	return fn(ctx)
}

package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Attempt is one provider call of a job (API view).
type Attempt struct {
	UUID            uuid.UUID      `json:"uuid"`
	AttemptNo       int32          `json:"attempt_no"`
	Operation       string         `json:"operation"`
	Trigger         string         `json:"trigger"`
	Outcome         string         `json:"outcome"`
	RequestSummary  map[string]any `json:"request_summary"`
	ResponseSummary map[string]any `json:"response_summary"`
	Error           string         `json:"error"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      time.Time      `json:"finished_at"`
	DurationMs      int32          `json:"duration_ms"`
}

// JobDetail is a job with its attempts (API view; payload references only).
type JobDetail struct {
	UUID          uuid.UUID      `json:"uuid"`
	AccountUUID   *uuid.UUID     `json:"account_uuid"`
	Direction     string         `json:"direction"`
	Provider      string         `json:"provider"`
	Kind          string         `json:"kind"`
	Status        string         `json:"status"`
	Priority      string         `json:"priority"`
	Attempts      int32          `json:"attempts"`
	MaxAttempts   int32          `json:"max_attempts"`
	Deliveries    int32          `json:"deliveries"`
	LastError     string         `json:"last_error"`
	ProviderRef   string         `json:"provider_ref"`
	Payload       map[string]any `json:"payload"`
	Result        map[string]any `json:"result"`
	RefType       string         `json:"ref_type"`
	RefUUID       *uuid.UUID     `json:"ref_uuid"`
	NextAttemptAt *time.Time     `json:"next_attempt_at"`
	SubmittedAt   *time.Time     `json:"submitted_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Retryable     bool           `json:"retryable"`
	AttemptsLog   []Attempt      `json:"attempt_log"`
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func uuidPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	v := uuid.UUID(u.Bytes)
	return &v
}

func objectOf(raw []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// CanRetry reports whether a staff retry applies to a job status.
func CanRetry(kind, direction, status string) bool {
	if direction != "outbound" || kind == "payment.refund" {
		return false
	}
	switch status {
	case StatusPending, StatusFailed, StatusNeedsVerification, StatusSubmitted:
		return true
	}
	return false
}

// Detail returns a job of the organization with its attempts.
func (r *Runner) Detail(ctx context.Context, orgID int64, id uuid.UUID) (JobDetail, error) {
	row, err := r.q.GetRunnerJobByUUID(ctx, db.GetRunnerJobByUUIDParams{OrganizationID: orgParam(orgID), Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return JobDetail{}, ErrNotFound
	}
	if err != nil {
		return JobDetail{}, err
	}
	attempts, err := r.q.ListIntegrationAttempts(ctx, db.ListIntegrationAttemptsParams{
		OrganizationID: orgParam(orgID), IntegrationJobID: row.ID,
	})
	if err != nil {
		return JobDetail{}, err
	}
	d := JobDetail{
		UUID: row.Uuid, AccountUUID: uuidPtr(row.AccountUuid), Direction: row.Direction, Provider: row.Provider,
		Kind: row.Kind, Status: row.Status, Priority: row.Priority, Attempts: row.Attempts, MaxAttempts: row.MaxAttempts,
		Deliveries: row.Deliveries, LastError: row.LastError, ProviderRef: row.ProviderRef, Payload: objectOf(row.Payload),
		Result: objectOf(row.Result), RefType: row.RefType, RefUUID: uuidPtr(row.RefUuid),
		NextAttemptAt: timePtr(row.NextAttemptAt), SubmittedAt: timePtr(row.SubmittedAt), CompletedAt: timePtr(row.CompletedAt),
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		Retryable:   CanRetry(row.Kind, row.Direction, row.Status),
		AttemptsLog: make([]Attempt, 0, len(attempts)),
	}
	for _, a := range attempts {
		d.AttemptsLog = append(d.AttemptsLog, Attempt{
			UUID: a.Uuid, AttemptNo: a.AttemptNo, Operation: a.Operation, Trigger: a.Trigger, Outcome: a.Outcome,
			RequestSummary: objectOf(a.RequestSummary), ResponseSummary: objectOf(a.ResponseSummary), Error: a.Error,
			StartedAt: a.StartedAt.Time.UTC(), FinishedAt: a.FinishedAt.Time.UTC(), DurationMs: a.DurationMs,
		})
	}
	return d, nil
}

// Package usecase is the generic integration job runner (TEC-880,
// docs/design/integration-jobs.md). It owns the integration_jobs state
// machine for runner jobs and records every provider call as an
// integration_attempts row:
//
//	pending ──Submit──▶ succeeded | submitted (accepted) | failed (definitive)
//	                    | pending (retryable failure, backoff)
//	                    | needs_verification (outcome unknown)
//	submitted / needs_verification ──CheckStatus──▶ succeeded | submitted
//	                    | pending (provider confirmed not done → submit again)
//	                    | failed (not done, not retryable)
//	                    | needs_verification (still unknown / unsupported)
//
// The rule: an unknown outcome is verified with CheckStatus first; the same
// work is never sent again unless the provider confirmed it was not done.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Job statuses (integration_jobs.status; the runner's names in brackets).
const (
	StatusPending           = "pending" // [pending_submit]
	StatusProcessing        = "processing"
	StatusSubmitted         = "submitted"
	StatusSucceeded         = "succeeded"
	StatusFailed            = "failed"
	StatusIgnored           = "ignored"
	StatusNeedsVerification = "needs_verification"
	// Held: the runner never calls the provider for these (a module moves
	// them on). blocked = required data missing; waiting_dependency = no
	// working adapter yet.
	StatusBlocked           = "blocked"
	StatusWaitingDependency = "waiting_dependency"
	// Staff resolutions (final): the work was filed through the official
	// manual channel, or confirmed with the provider by a person.
	StatusReportedManually = "reported_manually"
	StatusReconciled       = "reconciled"
)

// Priorities.
const (
	PriorityCritical = "critical"
	PriorityNormal   = "normal"
)

// Attempt operations and triggers.
const (
	OpSubmit      = "submit"
	OpCheckStatus = "check_status"

	TriggerWorker = "worker"
	TriggerSweep  = "sweep"
	TriggerRetry  = "retry"
	TriggerInline = "inline"
)

// ResourceJobs is the live list resource of integration jobs.
const ResourceJobs = "tenant.integration_jobs"

// DefaultMaxAttempts bounds the automatic provider calls of a job.
const DefaultMaxAttempts = 6

var (
	// ErrNotFound: unknown job (or another organization's).
	ErrNotFound = errors.New("integration job not found")
	// ErrNotRetryable: the job is done or not a runner job.
	ErrNotRetryable = errors.New("the job cannot be retried in its current state")
	// ErrBusy: another worker holds the job right now.
	ErrBusy = errors.New("the job is being processed; try again shortly")
)

// VerifyError: a retry could not confirm whether the provider did the work,
// so nothing was sent again (409).
type VerifyError struct {
	Outcome integrations.Outcome
	Message string
}

func (e *VerifyError) Error() string {
	if e.Message != "" {
		return "the provider did not confirm the job's state: " + e.Message
	}
	return "the provider did not confirm the job's state"
}

// AccountSource opens an organization's integration account by id.
type AccountSource interface {
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

// Dispatcher queues a job step (the worker's integrations queues). A nil
// dispatcher runs the first step inline; later steps wait for the sweep.
type Dispatcher interface {
	DispatchIntegrationJob(orgID, jobID int64, critical bool, delay time.Duration) error
}

// Listener hears every state change of a job (a message log follows its
// send jobs).
type Listener interface {
	IntegrationJobChanged(ctx context.Context, orgID int64, job State, res integrations.Result)
}

// State is a job after a step.
type State struct {
	ID          int64
	UUID        uuid.UUID
	Kind        string
	Provider    string
	Status      string
	ProviderRef string
	Attempts    int32
	LastError   string
	Operation   string
	// Payload is the job's reference payload (JSON object).
	Payload []byte
}

// EnqueueInput creates a runner job.
type EnqueueInput struct {
	OrganizationID int64
	AccountID      int64
	Provider       string
	Kind           string
	// IdempotencyKey is unique per provider: the same key never creates a
	// second job (callers prefix it with "org:<id>:").
	IdempotencyKey string
	Payload        map[string]any
	RefType        string
	RefUUID        *uuid.UUID
	Critical       bool
	MaxAttempts    int32
}

// Enqueued is the created (or existing) job.
type Enqueued struct {
	ID       int64
	UUID     uuid.UUID
	Status   string
	Inserted bool
}

// Runner runs integration jobs.
type Runner struct {
	q         *db.Queries
	accounts  AccountSource
	ops       *integrations.OperationRegistry
	dispatch  Dispatcher
	listeners []Listener
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
	timeout   time.Duration
	lease     time.Duration
}

// NewRunner builds the runner.
func NewRunner(q *db.Queries, accounts AccountSource, ops *integrations.OperationRegistry) *Runner {
	return &Runner{
		q: q, accounts: accounts, ops: ops, events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
		timeout: 30 * time.Second, lease: 2 * time.Minute,
	}
}

// SetDispatcher queues later steps (nil = inline first step only).
func (r *Runner) SetDispatcher(d Dispatcher) { r.dispatch = d }

// AddListener registers a state change listener.
func (r *Runner) AddListener(l Listener) {
	if l != nil {
		r.listeners = append(r.listeners, l)
	}
}

// SetResourceEvents publishes live list changes.
func (r *Runner) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		r.events = p
	}
}

// SetLogger replaces the logger.
func (r *Runner) SetLogger(l *slog.Logger) {
	if l != nil {
		r.log = l
	}
}

// SetClock replaces the clock (tests).
func (r *Runner) SetClock(now func() time.Time) { r.now = now }

// Backoff is the delay before automatic call n (1-based): 15s, 30s, 1m, 2m
// … capped at 30 minutes. Also the asynq retry delay of integration tasks.
func Backoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	d := 15 * time.Second * time.Duration(math.Pow(2, float64(n-1)))
	if d > 30*time.Minute || d <= 0 {
		d = 30 * time.Minute
	}
	return d
}

func orgParam(orgID int64) pgtype.Int8 {
	return pgtype.Int8{Int64: orgID, Valid: orgID > 0}
}

func jsonOf(v map[string]any) []byte {
	if len(v) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Enqueue creates the job once (idempotency key) and dispatches its first
// step. A repeated key returns the existing job without sending anything.
func (r *Runner) Enqueue(ctx context.Context, in EnqueueInput) (Enqueued, error) {
	row, err := r.insert(ctx, r.q, in)
	if err != nil {
		return Enqueued{}, err
	}
	out := Enqueued{ID: row.ID, UUID: row.Uuid, Status: row.Status, Inserted: row.Inserted}
	if !row.Inserted {
		return out, nil
	}
	r.publish(ctx, in.OrganizationID, row.Uuid)
	if r.dispatch != nil {
		if err := r.dispatch.DispatchIntegrationJob(in.OrganizationID, row.ID, in.Critical, 0); err == nil {
			return out, nil
		} else {
			r.log.Warn("integration_job_dispatch_failed", "job", row.Uuid, "error", err)
		}
	}
	// No queue: run the first step now; later steps wait for the sweep.
	st, err := r.Process(ctx, in.OrganizationID, row.ID, TriggerInline)
	if err != nil && !errors.Is(err, ErrBusy) {
		r.log.Warn("integration_job_inline_failed", "job", row.Uuid, "error", err)
	}
	if st.Status != "" {
		out.Status = st.Status
	}
	return out, nil
}

// EnqueueTx creates the job inside the caller's transaction, next to the
// business write it belongs to (outbox style): the job exists exactly when
// the change commits, so a provider call is never lost after a commit nor
// made for a rolled-back change. Nothing is dispatched here; call Kick
// after the commit (the minute sweep runs it anyway: next_attempt_at is
// set to now).
func (r *Runner) EnqueueTx(ctx context.Context, tx pgx.Tx, in EnqueueInput) (Enqueued, error) {
	row, err := r.insert(ctx, r.q.WithTx(tx), in)
	if err != nil {
		return Enqueued{}, err
	}
	return Enqueued{ID: row.ID, UUID: row.Uuid, Status: row.Status, Inserted: row.Inserted}, nil
}

func (r *Runner) insert(ctx context.Context, q *db.Queries, in EnqueueInput) (db.EnqueueIntegrationJobRow, error) {
	if in.Provider == "" || in.Kind == "" || in.IdempotencyKey == "" {
		return db.EnqueueIntegrationJobRow{}, errors.New("integrations runner: provider, kind and idempotency key are required")
	}
	if in.MaxAttempts <= 0 {
		in.MaxAttempts = DefaultMaxAttempts
	}
	priority := PriorityNormal
	if in.Critical {
		priority = PriorityCritical
	}
	p := db.EnqueueIntegrationJobParams{
		OrganizationID: orgParam(in.OrganizationID), Provider: in.Provider, Kind: in.Kind,
		IdempotencyKey: in.IdempotencyKey, Payload: jsonOf(in.Payload), RefType: in.RefType, Priority: priority,
		MaxAttempts: in.MaxAttempts,
	}
	if in.AccountID > 0 {
		p.IntegrationAccountID = pgtype.Int8{Int64: in.AccountID, Valid: true}
	}
	if in.RefUUID != nil {
		p.RefUuid = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
	}
	return q.EnqueueIntegrationJob(ctx, p)
}

// Kick starts the next step of a job a module created itself (outbox
// style, in its own transaction, e.g. a reservation reminder): the worker
// queue when a dispatcher is set, otherwise one inline step now. The
// sweep picks the job up anyway when neither happens (a crash).
func (r *Runner) Kick(ctx context.Context, orgID, jobID int64) (State, error) {
	r.publishID(ctx, orgID, jobID)
	if r.dispatch != nil {
		if err := r.dispatch.DispatchIntegrationJob(orgID, jobID, false, 0); err == nil {
			return State{ID: jobID}, nil
		} else {
			r.log.Warn("integration_job_dispatch_failed", "job_id", jobID, "error", err)
		}
	}
	st, err := r.Process(ctx, orgID, jobID, TriggerInline)
	if errors.Is(err, ErrBusy) {
		return st, nil
	}
	return st, err
}

func (r *Runner) publishID(ctx context.Context, orgID, jobID int64) {
	if orgID <= 0 {
		return
	}
	if j, err := r.load(ctx, orgID, jobID); err == nil {
		r.publish(ctx, orgID, j.Uuid)
	}
}

// Changed tells the listeners and the live list about a state change a
// module made outside a provider call (a staff resolution, a re-route of
// a held job).
func (r *Runner) Changed(ctx context.Context, orgID, jobID int64) {
	j, err := r.load(ctx, orgID, jobID)
	if err != nil {
		return
	}
	st := stateOf(j, "")
	for _, l := range r.listeners {
		l.IntegrationJobChanged(ctx, orgID, st, integrations.Result{})
	}
	r.publish(ctx, orgID, j.Uuid)
}

func (r *Runner) load(ctx context.Context, orgID, jobID int64) (db.GetRunnerJobRow, error) {
	j, err := r.q.GetRunnerJob(ctx, db.GetRunnerJobParams{OrganizationID: orgParam(orgID), ID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// Process runs the job's next step (worker task, sweep, inline). It returns
// the state after the step; a later step is dispatched with the backoff
// delay when a dispatcher is set (the sweep catches it otherwise).
func (r *Runner) Process(ctx context.Context, orgID, jobID int64, trigger string) (State, error) {
	if orgID > 0 {
		ctx = orgctx.WithOrganizationID(ctx, orgID)
	}
	j, err := r.load(ctx, orgID, jobID)
	if err != nil {
		return State{}, err
	}
	var op string
	switch j.Status {
	case StatusPending:
		op = OpSubmit
	case StatusSubmitted, StatusNeedsVerification:
		op = OpCheckStatus
	default:
		return stateOf(j, ""), nil
	}
	if j.Attempts >= j.MaxAttempts {
		// Automatic calls are used up: a person decides (retry).
		return stateOf(j, ""), nil
	}
	if err := r.lock(ctx, orgID, j); err != nil {
		return stateOf(j, ""), err
	}
	var next transition
	if op == OpSubmit {
		res := r.call(ctx, orgID, j, OpSubmit, trigger)
		next = afterSubmit(j, res)
		next.res = res
	} else {
		res := r.call(ctx, orgID, j, OpCheckStatus, trigger)
		next = afterCheck(j, res)
		next.res = res
	}
	st, err := r.save(ctx, orgID, j, next)
	if err != nil {
		return st, err
	}
	r.schedule(orgID, j, next)
	return st, nil
}

func (r *Runner) lock(ctx context.Context, orgID int64, j db.GetRunnerJobRow) error {
	n, err := r.q.LockRunnerJob(ctx, db.LockRunnerJobParams{
		OrganizationID: orgParam(orgID), ID: j.ID, LeaseSeconds: int32(r.lease / time.Second), Statuses: []string{j.Status},
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrBusy
	}
	return nil
}

func stateOf(j db.GetRunnerJobRow, op string) State {
	return State{
		ID: j.ID, UUID: j.Uuid, Kind: j.Kind, Provider: j.Provider, Status: j.Status, ProviderRef: j.ProviderRef,
		Attempts: j.Attempts, LastError: j.LastError, Operation: op, Payload: j.Payload,
	}
}

func opJob(orgID int64, j db.GetRunnerJobRow) integrations.Job {
	return integrations.Job{
		ID: j.ID, OrganizationID: orgID, Kind: j.Kind, Provider: j.Provider, IdempotencyKey: j.IdempotencyKey,
		Payload: j.Payload, ProviderRef: j.ProviderRef, Status: j.Status, Attempts: j.Attempts,
	}
}

// call performs one provider call, classifies it and records the attempt.
// Errors never escape as success: an unclassified error is uncertain, an
// unsupported operation is explicit.
func (r *Runner) call(ctx context.Context, orgID int64, j db.GetRunnerJobRow, op, trigger string) (res integrations.Result) {
	started := r.now()
	attemptNo := j.Attempts + 1
	defer func() {
		finished := r.now()
		if finished.Before(started) {
			finished = started
		}
		if _, err := r.q.InsertIntegrationAttempt(ctx, db.InsertIntegrationAttemptParams{
			OrganizationID: orgParam(orgID), IntegrationJobID: j.ID, AttemptNo: attemptNo, Operation: op, Trigger: trigger,
			Outcome: string(res.Outcome), RequestSummary: jsonOf(res.Request), ResponseSummary: jsonOf(res.Response),
			Error: trim(res.Message, 1000), StartedAt: pgtype.Timestamptz{Time: started, Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: finished, Valid: true}, DurationMs: int32(finished.Sub(started) / time.Millisecond),
		}); err != nil {
			r.log.Warn("integration_attempt_record_failed", "job", j.Uuid, "error", err)
		}
	}()
	// A failed job without a provider reference was rejected by the
	// provider's own definitive answer (failed is never "unknown"): that
	// recorded answer is the status — nothing was queued at the provider.
	if op == OpCheckStatus && j.Status == StatusFailed && j.ProviderRef == "" {
		return integrations.Result{
			Outcome: integrations.OutcomeNotFound, Response: map[string]any{"source": "recorded_provider_answer"},
			Message: "the provider rejected the previous submit; nothing was queued",
		}
	}
	operation, ok := r.ops.Get(j.Provider, j.Kind)
	if !ok {
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Message: "no operation " + j.Kind + " for provider " + j.Provider}
	}
	if !j.IntegrationAccountID.Valid || orgID <= 0 {
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Message: "the job has no integration account"}
	}
	acc, err := r.accounts.AccountByID(ctx, orgID, j.IntegrationAccountID.Int64)
	if err != nil {
		// Nothing reached the provider.
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: "account: " + err.Error()}
	}
	cctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var callErr error
	if op == OpSubmit {
		res, callErr = operation.Submit(cctx, acc, opJob(orgID, j))
	} else {
		res, callErr = operation.CheckStatus(cctx, acc, opJob(orgID, j))
	}
	switch {
	case errors.Is(callErr, integrations.ErrUnsupported):
		res.Outcome = integrations.OutcomeUnsupported
		if res.Message == "" {
			res.Message = callErr.Error()
		}
	case callErr != nil && (res.Outcome == "" || res.Outcome == integrations.OutcomeSucceeded ||
		res.Outcome == integrations.OutcomeAccepted || res.Outcome == integrations.OutcomePending):
		// An error with a "done" or empty classification is not trusted.
		res.Outcome = integrations.OutcomeUncertain
		res.Message = callErr.Error()
	case callErr != nil && res.Message == "":
		res.Message = callErr.Error()
	case res.Outcome == "":
		res.Outcome = integrations.OutcomeUncertain
		res.Message = "the operation returned no outcome"
	}
	if op == OpSubmit && res.Outcome == integrations.OutcomeAccepted && res.ProviderRef == "" {
		// Accepted without a reference cannot be checked later.
		res.Outcome = integrations.OutcomeSucceeded
	}
	return res
}

type transition struct {
	status      string
	providerRef string
	lastError   string
	next        time.Duration // < 0: no automatic next step
	submitted   bool
	maxAttempts int32
	res         integrations.Result
	op          string
}

const pollStart = 30 * time.Second

func afterSubmit(j db.GetRunnerJobRow, res integrations.Result) transition {
	t := transition{status: j.Status, providerRef: j.ProviderRef, next: -1, maxAttempts: j.MaxAttempts, op: OpSubmit}
	attempts := int(j.Attempts) + 1
	switch res.Outcome {
	case integrations.OutcomeSucceeded:
		t.status, t.submitted = StatusSucceeded, true
		if res.ProviderRef != "" {
			t.providerRef = res.ProviderRef
		}
	case integrations.OutcomeAccepted:
		t.status, t.providerRef, t.submitted, t.next = StatusSubmitted, res.ProviderRef, true, pollStart
	case integrations.OutcomeFailed:
		t.lastError = res.Message
		if res.Retryable && int32(attempts) < j.MaxAttempts {
			t.status, t.next = StatusPending, Backoff(attempts)
		} else {
			t.status = StatusFailed
		}
	case integrations.OutcomeUnsupported:
		t.status, t.lastError = StatusFailed, "unsupported: "+res.Message
	default: // uncertain
		t.status, t.lastError, t.next = StatusNeedsVerification, res.Message, Backoff(attempts)
	}
	if t.next >= 0 && int32(attempts) >= j.MaxAttempts {
		t.next = -1
	}
	return t
}

func afterCheck(j db.GetRunnerJobRow, res integrations.Result) transition {
	t := transition{status: j.Status, providerRef: j.ProviderRef, lastError: j.LastError, next: -1, maxAttempts: j.MaxAttempts, op: OpCheckStatus}
	attempts := int(j.Attempts) + 1
	switch res.Outcome {
	case integrations.OutcomeSucceeded:
		t.status, t.lastError = StatusSucceeded, ""
		if res.ProviderRef != "" {
			// The status check found the work under its reference.
			t.providerRef = res.ProviderRef
		}
	case integrations.OutcomeAccepted, integrations.OutcomePending:
		t.status, t.lastError, t.next = StatusSubmitted, "", Backoff(attempts)+pollStart
		if res.ProviderRef != "" {
			t.providerRef = res.ProviderRef
		}
	case integrations.OutcomeNotFound, integrations.OutcomeFailed:
		t.lastError = res.Message
		if res.Outcome == integrations.OutcomeFailed && !res.Retryable {
			t.status = StatusFailed
		} else if int32(attempts) < j.MaxAttempts {
			// Confirmed not done: submitting again is safe.
			t.status, t.providerRef, t.next = StatusPending, "", 0
		} else {
			t.status = StatusFailed
		}
	case integrations.OutcomeUnsupported:
		// The provider cannot tell: a person must decide; never resend.
		t.status, t.lastError = StatusNeedsVerification, "status check unsupported: "+res.Message
	default:
		t.status, t.lastError, t.next = StatusNeedsVerification, res.Message, Backoff(attempts)
	}
	if t.next > 0 && int32(attempts) >= j.MaxAttempts {
		t.next = -1
	}
	return t
}

func (r *Runner) save(ctx context.Context, orgID int64, j db.GetRunnerJobRow, t transition) (State, error) {
	p := db.SaveRunnerJobParams{
		OrganizationID: orgParam(orgID), ID: j.ID, Status: t.status, Attempts: j.Attempts + 1, MaxAttempts: t.maxAttempts,
		ProviderRef: t.providerRef, LastError: trim(t.lastError, 1000), Result: jsonOf(t.res.Response),
	}
	if t.next >= 0 {
		p.NextAttemptAt = pgtype.Timestamptz{Time: r.now().Add(t.next), Valid: true}
	}
	if t.submitted {
		p.SubmittedAt = pgtype.Timestamptz{Time: r.now(), Valid: true}
	}
	if err := r.q.SaveRunnerJob(ctx, p); err != nil {
		_ = r.q.UnlockRunnerJob(ctx, db.UnlockRunnerJobParams{OrganizationID: orgParam(orgID), ID: j.ID})
		return stateOf(j, t.op), err
	}
	st := State{
		ID: j.ID, UUID: j.Uuid, Kind: j.Kind, Provider: j.Provider, Status: t.status, ProviderRef: t.providerRef,
		Attempts: j.Attempts + 1, LastError: t.lastError, Operation: t.op, Payload: j.Payload,
	}
	for _, l := range r.listeners {
		l.IntegrationJobChanged(ctx, orgID, st, t.res)
	}
	r.publish(ctx, orgID, j.Uuid)
	if t.status == StatusNeedsVerification || t.status == StatusFailed {
		r.log.Warn("integration_job_attention", "job", j.Uuid, "kind", j.Kind, "status", t.status, "error", trim(t.lastError, 200))
	}
	return st, nil
}

func (r *Runner) schedule(orgID int64, j db.GetRunnerJobRow, t transition) {
	if r.dispatch == nil || t.next < 0 {
		return
	}
	if err := r.dispatch.DispatchIntegrationJob(orgID, j.ID, j.Priority == PriorityCritical, t.next); err != nil {
		r.log.Warn("integration_job_dispatch_failed", "job", j.Uuid, "error", err)
	}
}

func (r *Runner) publish(ctx context.Context, orgID int64, id uuid.UUID) {
	if orgID <= 0 {
		return
	}
	r.events.Publish(orgctx.WithOrganizationID(ctx, orgID), ResourceJobs, "updated", id.String())
}

// RunDue is the sweep: every runner job whose next step is due (a lost
// queue task, delivery report polling) gets one step. It returns the
// number of steps run.
func (r *Runner) RunDue(ctx context.Context) (int, error) {
	rows, err := r.q.ListDueRunnerJobs(ctx, 200)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		if _, err := r.Process(ctx, row.OrganizationID.Int64, row.ID, TriggerSweep); err != nil {
			if !errors.Is(err, ErrBusy) {
				r.log.Warn("integration_job_sweep_failed", "job_id", row.ID, "error", err)
			}
			continue
		}
		n++
	}
	return n, nil
}

// RetryResult says what a staff retry did.
type RetryResult struct {
	// already_done | in_progress | resubmitted | dispatched
	Action string `json:"action"`
	Status string `json:"status"`
}

// Retry is the staff "Yeniden dene": CheckStatus first; the work is
// submitted again only when the provider confirms it was not done. An
// unknown or unsupported status answers *VerifyError and sends nothing.
func (r *Runner) Retry(ctx context.Context, orgID int64, id uuid.UUID) (RetryResult, error) {
	row, err := r.q.GetRunnerJobByUUID(ctx, db.GetRunnerJobByUUIDParams{OrganizationID: orgParam(orgID), Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RetryResult{}, ErrNotFound
	}
	if err != nil {
		return RetryResult{}, err
	}
	j := db.GetRunnerJobRow(row)
	if !CanRetry(j.Kind, j.Direction, j.Status) {
		return RetryResult{}, ErrNotRetryable
	}
	switch j.Status {
	case StatusPending:
		// Nothing was sent yet (or the provider confirmed it was not done):
		// run the submit now.
		if j.Attempts >= j.MaxAttempts {
			j.MaxAttempts = j.Attempts + 3
			if err := r.q.SaveRunnerJob(ctx, db.SaveRunnerJobParams{
				OrganizationID: orgParam(orgID), ID: j.ID, Status: j.Status, Attempts: j.Attempts, MaxAttempts: j.MaxAttempts,
				ProviderRef: j.ProviderRef, LastError: j.LastError, Result: j.Result,
				NextAttemptAt: pgtype.Timestamptz{Time: r.now(), Valid: true},
			}); err != nil {
				return RetryResult{}, err
			}
		}
		st, err := r.Process(ctx, orgID, j.ID, TriggerRetry)
		return RetryResult{Action: "resubmitted", Status: st.Status}, err
	case StatusFailed, StatusNeedsVerification, StatusSubmitted:
	default:
		return RetryResult{}, ErrNotRetryable
	}
	if orgID > 0 {
		ctx = orgctx.WithOrganizationID(ctx, orgID)
	}
	if err := r.lock(ctx, orgID, j); err != nil {
		return RetryResult{}, err
	}
	// A staff retry may use calls beyond the automatic limit.
	if j.MaxAttempts < j.Attempts+3 {
		j.MaxAttempts = j.Attempts + 3
	}
	res := r.call(ctx, orgID, j, OpCheckStatus, TriggerRetry)
	switch {
	case res.Outcome == integrations.OutcomeSucceeded:
		ref := j.ProviderRef
		if res.ProviderRef != "" {
			ref = res.ProviderRef
		}
		t := transition{status: StatusSucceeded, providerRef: ref, next: -1, maxAttempts: j.MaxAttempts, res: res, op: OpCheckStatus}
		st, err := r.save(ctx, orgID, j, t)
		return RetryResult{Action: "already_done", Status: st.Status}, err
	case res.Outcome == integrations.OutcomePending || res.Outcome == integrations.OutcomeAccepted:
		ref := j.ProviderRef
		if res.ProviderRef != "" {
			ref = res.ProviderRef
		}
		t := transition{status: StatusSubmitted, providerRef: ref, next: pollStart, maxAttempts: j.MaxAttempts, res: res, op: OpCheckStatus}
		st, err := r.save(ctx, orgID, j, t)
		r.schedule(orgID, j, t)
		return RetryResult{Action: "in_progress", Status: st.Status}, err
	case res.Outcome.NotDone():
		// Confirmed not done: record the check, then submit again.
		t := transition{status: StatusPending, next: 0, maxAttempts: j.MaxAttempts, lastError: res.Message, res: res, op: OpCheckStatus}
		if _, err := r.save(ctx, orgID, j, t); err != nil {
			return RetryResult{}, err
		}
		fresh, err := r.load(ctx, orgID, j.ID)
		if err != nil {
			return RetryResult{}, err
		}
		if err := r.lock(ctx, orgID, fresh); err != nil {
			return RetryResult{}, err
		}
		sres := r.call(ctx, orgID, fresh, OpSubmit, TriggerRetry)
		next := afterSubmit(fresh, sres)
		next.res = sres
		st, err := r.save(ctx, orgID, fresh, next)
		if err == nil {
			r.schedule(orgID, fresh, next)
		}
		return RetryResult{Action: "resubmitted", Status: st.Status}, err
	default:
		// Unknown / unsupported: keep (or move to) needs_verification.
		status := j.Status
		if status != StatusSubmitted {
			status = StatusNeedsVerification
		}
		msg := res.Message
		t := transition{status: status, providerRef: j.ProviderRef, lastError: msg, next: -1, maxAttempts: j.MaxAttempts, res: res, op: OpCheckStatus}
		if _, err := r.save(ctx, orgID, j, t); err != nil {
			return RetryResult{}, err
		}
		return RetryResult{}, &VerifyError{Outcome: res.Outcome, Message: msg}
	}
}

// Describe is a short text of a state for logs.
func (s State) Describe() string {
	return fmt.Sprintf("%s %s (%s)", s.Kind, s.Status, s.UUID)
}

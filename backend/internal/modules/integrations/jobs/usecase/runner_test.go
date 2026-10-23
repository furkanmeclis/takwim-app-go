package usecase_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations/logprovider"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeOp is a scripted operation: every call pops the next result of its
// list (the last one repeats) and counts the calls.
type fakeOp struct {
	mu       sync.Mutex
	submits  []integrations.Result
	checks   []integrations.Result
	checkErr error
	nSubmit  int
	nCheck   int
	done     bool // the provider really did the work
}

func (f *fakeOp) Kind() string { return "test.op" }

func pop(list []integrations.Result, n int) integrations.Result {
	if n >= len(list) {
		return list[len(list)-1]
	}
	return list[n]
}

func (f *fakeOp) Submit(_ context.Context, _ integrations.Account, _ integrations.Job) (integrations.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := pop(f.submits, f.nSubmit)
	f.nSubmit++
	if r.Outcome == integrations.OutcomeUncertain {
		// The request reached the provider; only the answer was lost.
		f.done = true
		return r, errors.New("connection reset")
	}
	if r.Outcome == integrations.OutcomeAccepted || r.Outcome == integrations.OutcomeSucceeded {
		f.done = true
	}
	return r, nil
}

func (f *fakeOp) CheckStatus(_ context.Context, _ integrations.Account, _ integrations.Job) (integrations.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nCheck++
	if f.checkErr != nil {
		return integrations.Result{}, f.checkErr
	}
	if len(f.checks) > 0 {
		return pop(f.checks, f.nCheck-1), nil
	}
	if f.done {
		return integrations.Result{Outcome: integrations.OutcomeSucceeded}, nil
	}
	return integrations.Result{Outcome: integrations.OutcomeNotFound}, nil
}

type env struct {
	ctx    context.Context
	q      *db.Queries
	pool   *pgxpool.Pool
	org    int64
	acc    int64
	runner *usecase.Runner
	op     *fakeOp
}

func newEnv(t *testing.T, op *fakeOp) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "runner-" + suffix, Name: "Runner " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID) })
	fc, err := crypto.NewFieldCipher(strings.Repeat("r", 32))
	if err != nil {
		t.Fatal(err)
	}
	vault, err := integrations.NewVault(fc)
	if err != nil {
		t.Fatal(err)
	}
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(logprovider.New(integrations.KindSMS, nil)))
	a, err := accounts.Connect(ctx, org.ID, accountsusecase.ConnectInput{
		Provider: "log", DisplayName: "Test", Credentials: map[string]string{"sender": "TAKWIM"},
	})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := accounts.AccountByUUID(ctx, org.ID, a.UUID)
	if err != nil {
		t.Fatal(err)
	}
	ops := integrations.NewOperationRegistry()
	ops.Register("log", op)
	return &env{ctx: ctx, q: q, pool: pool, org: org.ID, acc: acc.ID, runner: usecase.NewRunner(q, accounts, ops), op: op}
}

func (e *env) enqueue(t *testing.T) usecase.Enqueued {
	t.Helper()
	j, err := e.runner.Enqueue(e.ctx, usecase.EnqueueInput{
		OrganizationID: e.org, AccountID: e.acc, Provider: "log", Kind: "test.op",
		IdempotencyKey: "org:test:" + uuid.NewString(), Payload: map[string]any{"ref": "x"}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func (e *env) detail(t *testing.T, id uuid.UUID) usecase.JobDetail {
	t.Helper()
	d, err := e.runner.Detail(e.ctx, e.org, id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// An uncertain submit goes to needs_verification; the next step asks
// CheckStatus and never submits again when the provider did the work.
func TestRunnerUncertainChecksStatusWithoutDoubleSubmit(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeUncertain}}}
	e := newEnv(t, op)
	j := e.enqueue(t) // no dispatcher: the first step runs inline
	d := e.detail(t, j.UUID)
	if d.Status != usecase.StatusNeedsVerification {
		t.Fatalf("after uncertain submit: status %s, want needs_verification", d.Status)
	}
	if _, err := e.runner.Process(e.ctx, e.org, jobID(t, e, j.UUID), usecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	d = e.detail(t, j.UUID)
	if d.Status != usecase.StatusSucceeded {
		t.Fatalf("after check: status %s, want succeeded", d.Status)
	}
	if op.nSubmit != 1 || op.nCheck != 1 {
		t.Fatalf("submits=%d checks=%d, want 1 / 1", op.nSubmit, op.nCheck)
	}
	if len(d.AttemptsLog) != 2 || d.AttemptsLog[0].Operation != "submit" || d.AttemptsLog[0].Outcome != "uncertain" ||
		d.AttemptsLog[1].Operation != "check_status" || d.AttemptsLog[1].Outcome != "succeeded" {
		t.Fatalf("attempt log: %+v", d.AttemptsLog)
	}
	// Further steps and a staff retry do nothing more.
	if _, err := e.runner.Process(e.ctx, e.org, jobID(t, e, j.UUID), usecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runner.Retry(e.ctx, e.org, j.UUID); !errors.Is(err, usecase.ErrNotRetryable) {
		t.Fatalf("retry of a succeeded job: %v", err)
	}
	if op.nSubmit != 1 {
		t.Fatalf("submitted again: %d", op.nSubmit)
	}
}

// When CheckStatus confirms the work was not done, the job is submitted
// again (exactly once).
func TestRunnerResubmitsOnlyWhenConfirmedNotDone(t *testing.T) {
	op := &fakeOp{
		submits: []integrations.Result{{Outcome: integrations.OutcomeUncertain}, {Outcome: integrations.OutcomeSucceeded}},
		checks:  []integrations.Result{{Outcome: integrations.OutcomeNotFound}},
	}
	e := newEnv(t, op)
	j := e.enqueue(t)
	id := jobID(t, e, j.UUID)
	st, err := e.runner.Process(e.ctx, e.org, id, usecase.TriggerSweep) // check → not found → pending
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != usecase.StatusPending {
		t.Fatalf("after not_found: %s", st.Status)
	}
	if op.nSubmit != 1 {
		t.Fatalf("check must not submit: %d", op.nSubmit)
	}
	st, err = e.runner.Process(e.ctx, e.org, id, usecase.TriggerSweep)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != usecase.StatusSucceeded || op.nSubmit != 2 {
		t.Fatalf("resubmit: status %s submits %d", st.Status, op.nSubmit)
	}
}

// A status the provider cannot report keeps the job in needs_verification
// for a person: a staff retry answers VerifyError and sends nothing.
func TestRunnerUnsupportedCheckNeverResends(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeUncertain}}}
	op.checkErr = integrations.ErrUnsupported
	e := newEnv(t, op)
	j := e.enqueue(t)
	if _, err := e.runner.Process(e.ctx, e.org, jobID(t, e, j.UUID), usecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	d := e.detail(t, j.UUID)
	if d.Status != usecase.StatusNeedsVerification || d.NextAttemptAt != nil {
		t.Fatalf("unsupported check: status %s next %v", d.Status, d.NextAttemptAt)
	}
	_, err := e.runner.Retry(e.ctx, e.org, j.UUID)
	var ve *usecase.VerifyError
	if !errors.As(err, &ve) {
		t.Fatalf("retry: %v, want VerifyError", err)
	}
	if op.nSubmit != 1 {
		t.Fatalf("resent without confirmation: %d submits", op.nSubmit)
	}
	if d := e.detail(t, j.UUID); d.Status != usecase.StatusNeedsVerification {
		t.Fatalf("after retry: %s", d.Status)
	}
}

// A definitive rejection fails the job; the staff retry checks first (the
// recorded answer: nothing queued) and submits again.
func TestRunnerRetryAfterDefinitiveFailure(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{
		{Outcome: integrations.OutcomeFailed, Message: "bad header"},
		{Outcome: integrations.OutcomeAccepted, ProviderRef: "job-1"},
	}}
	e := newEnv(t, op)
	j := e.enqueue(t)
	if d := e.detail(t, j.UUID); d.Status != usecase.StatusFailed || !d.Retryable {
		t.Fatalf("after rejection: %s retryable=%v", d.Status, d.Retryable)
	}
	res, err := e.runner.Retry(e.ctx, e.org, j.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "resubmitted" || res.Status != usecase.StatusSubmitted {
		t.Fatalf("retry: %+v", res)
	}
	d := e.detail(t, j.UUID)
	if d.ProviderRef != "job-1" || len(d.AttemptsLog) != 3 || d.AttemptsLog[1].Operation != "check_status" ||
		d.AttemptsLog[1].Trigger != "retry" || d.AttemptsLog[2].Operation != "submit" {
		t.Fatalf("attempts: %+v", d.AttemptsLog)
	}
	// An accepted job is then checked (delivery report) → succeeded.
	if _, err := e.runner.Process(e.ctx, e.org, jobID(t, e, j.UUID), usecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if d := e.detail(t, j.UUID); d.Status != usecase.StatusSucceeded {
		t.Fatalf("after report: %s", d.Status)
	}
}

// Unknown operations fail explicitly; retryable failures back off and stop
// at max_attempts; the same idempotency key never creates a second job.
func TestRunnerUnsupportedAndLimits(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeFailed, Retryable: true, Message: "busy"}}}
	e := newEnv(t, op)
	j, err := e.runner.Enqueue(e.ctx, usecase.EnqueueInput{
		OrganizationID: e.org, AccountID: e.acc, Provider: "log", Kind: "test.unknown", IdempotencyKey: "org:t:" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	d := e.detail(t, j.UUID)
	if d.Status != usecase.StatusFailed || !strings.Contains(d.LastError, "unsupported") || d.AttemptsLog[0].Outcome != "unsupported" {
		t.Fatalf("unknown op: %+v", d)
	}

	key := "org:t:" + uuid.NewString()
	in := usecase.EnqueueInput{OrganizationID: e.org, AccountID: e.acc, Provider: "log", Kind: "test.op", IdempotencyKey: key, MaxAttempts: 3}
	first, err := e.runner.Enqueue(e.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.runner.Enqueue(e.ctx, in)
	if err != nil || again.Inserted || again.UUID != first.UUID {
		t.Fatalf("idempotency: %+v %v", again, err)
	}
	id := jobID(t, e, first.UUID)
	d = e.detail(t, first.UUID)
	if d.Status != usecase.StatusPending || d.NextAttemptAt == nil {
		t.Fatalf("retryable failure: %+v", d)
	}
	for range 5 {
		if _, err := e.runner.Process(e.ctx, e.org, id, usecase.TriggerSweep); err != nil {
			t.Fatal(err)
		}
	}
	d = e.detail(t, first.UUID)
	if d.Status != usecase.StatusFailed || d.Attempts != 3 || op.nSubmit != 3 {
		t.Fatalf("limit: status %s attempts %d submits %d", d.Status, d.Attempts, op.nSubmit)
	}
	if usecase.Backoff(1) >= usecase.Backoff(2) || usecase.Backoff(20) != usecase.Backoff(30) {
		t.Fatal("backoff must grow and cap")
	}
}

func TestRunnerOtherOrganizationCannotSeeJob(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeSucceeded}}}
	e := newEnv(t, op)
	j := e.enqueue(t)
	other := newEnv(t, op)
	if _, err := other.runner.Detail(other.ctx, other.org, j.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("detail from another organization: %v", err)
	}
	if _, err := other.runner.Retry(other.ctx, other.org, j.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("retry from another organization: %v", err)
	}
}

func jobID(t *testing.T, e *env, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := e.pool.QueryRow(e.ctx, "SELECT id FROM integration_jobs WHERE organization_id = $1 AND uuid = $2", e.org, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

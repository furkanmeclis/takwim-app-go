package integrations

import (
	"context"
	"errors"
	"fmt"
)

// Operation is the contract of one kind of outbound work a provider does
// for us (TEC-880, docs/design/integration-jobs.md): sending an SMS or a
// WhatsApp message, refunding a payment. The generic runner stores the job,
// calls Submit, records every call as an integration_attempts row and
// decides what happens next from the Result.
//
// The rule: when the outcome of a call is not known —
// a timeout, a dropped connection, a 5xx after the request left — the job
// goes to needs_verification and only CheckStatus may move it on. The runner
// never sends the same work again unless CheckStatus confirmed that the
// provider did not do it. An operation an adapter cannot perform returns
// ErrUnsupported; the job then fails (Submit) or stays in
// needs_verification for a person (CheckStatus). Work that was not done
// never looks successful.
type Operation interface {
	// Kind is the job kind this operation runs ("sms.send").
	Kind() string
	// Submit performs the work once. It must classify its result: a
	// definitive provider answer (accepted / done / rejected) or
	// OutcomeUncertain when the request may have reached the provider.
	Submit(ctx context.Context, account Account, job Job) (Result, error)
	// CheckStatus asks the provider what happened to the work (by
	// job.ProviderRef, or the job's own reference when the provider
	// supports lookups by it). OutcomeNotFound / OutcomeFailed mean "not
	// done": only then may the runner submit again.
	CheckStatus(ctx context.Context, account Account, job Job) (Result, error)
}

// Job is what an operation sees of an integration_jobs row.
type Job struct {
	ID             int64
	OrganizationID int64
	Kind           string
	Provider       string
	IdempotencyKey string
	// Payload holds references and non-secret facts (JSON object). The
	// operation resolves anything sensitive (an SMS body) from its own
	// tables or a resolver; payloads never carry secrets.
	Payload     []byte
	ProviderRef string
	Status      string
	Attempts    int32
}

// Outcome classifies one provider call.
type Outcome string

// Outcomes. Submit answers Succeeded, Accepted, Failed or Uncertain;
// CheckStatus answers Succeeded, Pending (accepted, not final yet),
// Failed, NotFound or Uncertain.
const (
	// OutcomeSucceeded: the work is done (final).
	OutcomeSucceeded Outcome = "succeeded"
	// OutcomeAccepted: the provider queued the work; the final state comes
	// from CheckStatus (an SMS waiting for its delivery report).
	OutcomeAccepted Outcome = "accepted"
	// OutcomePending: CheckStatus — accepted, still not final.
	OutcomePending Outcome = "pending"
	// OutcomeFailed: the provider answered that the work was not done
	// (rejected, undeliverable). Retryable says whether sending again may
	// help (a provider-side temporary error) or not (bad credentials).
	OutcomeFailed Outcome = "failed"
	// OutcomeNotFound: CheckStatus — the provider has no record of the
	// work: it was not done.
	OutcomeNotFound Outcome = "not_found"
	// OutcomeUncertain: the result is unknown; verify before anything else.
	OutcomeUncertain Outcome = "uncertain"
	// OutcomeUnsupported: the adapter cannot perform the operation.
	OutcomeUnsupported Outcome = "unsupported"
)

// NotDone reports whether the outcome confirms the work was not done.
func (o Outcome) NotDone() bool { return o == OutcomeFailed || o == OutcomeNotFound }

// Result is the classified answer of one provider call.
type Result struct {
	Outcome Outcome
	// ProviderRef identifies the work at the provider (Netgsm jobid).
	ProviderRef string
	// Retryable: a failed submit may be sent again (temporary provider
	// error). Ignored for other outcomes.
	Retryable bool
	// Request / Response are short summaries for the attempt row:
	// references, codes and non-secret facts only.
	Request  map[string]any
	Response map[string]any
	// Message is a human-readable reason (provider code text).
	Message string
}

// ErrUnsupported: the adapter does not support the operation (explicit,
// never a silent success).
var ErrUnsupported = errors.New("integrations: unsupported operation")

// ErrUncertain wraps a transport failure whose effect is unknown.
var ErrUncertain = errors.New("integrations: outcome unknown")

// Uncertain builds an OutcomeUncertain result for err.
func Uncertain(err error, request map[string]any) (Result, error) {
	return Result{Outcome: OutcomeUncertain, Request: request, Message: err.Error()}, fmt.Errorf("%w: %v", ErrUncertain, err)
}

// OperationRegistry holds the operations by provider and job kind.
type OperationRegistry struct {
	ops map[string]Operation
}

// NewOperationRegistry builds an empty registry.
func NewOperationRegistry() *OperationRegistry {
	return &OperationRegistry{ops: map[string]Operation{}}
}

// Register adds the operation of provider (nil is skipped).
func (r *OperationRegistry) Register(provider string, op Operation) {
	if r == nil || op == nil {
		return
	}
	r.ops[provider+"/"+op.Kind()] = op
}

// Get returns the operation for a provider and job kind.
func (r *OperationRegistry) Get(provider, kind string) (Operation, bool) {
	if r == nil {
		return nil, false
	}
	op, ok := r.ops[provider+"/"+kind]
	return op, ok
}

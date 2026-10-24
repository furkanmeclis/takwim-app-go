// Package usecase is the automatic voice call of the confirmation requests
// (TEC-923, docs/design/confirmations.md "Voice call"): the customer
// notification channel that calls the customer through the business's own
// voice provider account (Channel) and reads out the visit with three keys
// (1 Geleceğim, 2 cancel, 3 change request), and the webhook of the account
// (Service.HandleWebhook) that turns the call's result into a delivery
// report and the key into an answer.
//
//   - The call being picked up is never a confirmation: a call answered
//     without a key or with another key is recorded as reached
//     (delivered_at) without an answer, the rule's next channel runs, and
//     the call is made again when the rule allows (CallMissed). A call not
//     answered, busy or unreachable fails the attempt the same way.
//   - A key acts only when the result belongs to the call we made (the
//     signed webhook address names the attempt, the provider's order id
//     must match), the number is the customer's and the request is the
//     booking's current one (an older request of a moved visit never acts).
//     Then customerpanel.ChannelAnswer applies it with the visit link's
//     rules: Geleceğim and change through the confirmation engine, cancel
//     through the ordinary cancel under the business's cancel cutoff (a paid
//     booking waits for its refund there).
//   - Each result is handled once (integration inbound jobs keyed by the
//     provider's order id and the attempt); an internal failure answers 500
//     so the provider delivers it again.
package usecase

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	confirmationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	customerpanelusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/voice/netgsm"
)

// InboundKind is the integration job kind of a call result.
const InboundKind = "voice_call.result"

// WebhookPurpose separates the key of the webhook tokens.
const WebhookPurpose = "voice.webhook"

// Answers of a key (customer_notification_responses.answer).
const (
	AnswerUnknown = "unknown"
)

// Results of a key.
const (
	ResultApplied  = "applied"
	ResultRejected = "rejected"
	ResultIgnored  = "ignored"
)

// Reasons of a key that was not applied.
const (
	ReasonNotConfirmation = "not_confirmation_request"
	ReasonOtherNumber     = "other_number"
	// ReasonInvalidKey: a key other than 1, 2 or 3.
	ReasonInvalidKey = "invalid_key"
	ReasonOutdated   = "request_outdated"
	ReasonNotFound   = "reservation_not_found"
	ReasonNotOpen    = "not_open"
)

// Errors of a call result (the attempt's error text).
const (
	ErrNoKey       = "answered, no key pressed"
	ErrInvalidKey  = "answered, invalid key"
	ErrNotAnswered = "not answered"
	ErrUnreachable = "unreachable"
	ErrBusy        = "busy"
	ErrCallFailed  = "call failed"
)

var (
	// ErrUnknownAccount: the webhook names no voice account of ours.
	ErrUnknownAccount = errors.New("unknown voice account")
	// ErrSignature: the webhook address is not one we gave out.
	ErrSignature = errors.New("invalid webhook token")
	// ErrMalformed: the body is not a call result.
	ErrMalformed = errors.New("malformed call result")
)

// KeyAnswers maps the keys of the call to answers.
var KeyAnswers = map[int]string{
	1: confirmationsmodel.AnswerConfirm,
	2: confirmationsmodel.AnswerCancel,
	3: confirmationsmodel.AnswerChange,
}

// KeyTexts are what the call says back after each key, in Turkish (the
// customer's language of the call). The key is only taken here: the answer
// is applied when the result arrives, so the texts promise nothing the
// business's rules could refuse.
var KeyTexts = map[int]string{
	1: "Teşekkür ederiz, randevunuza geleceğinizi kaydediyoruz. İyi günler.",
	2: "İptal isteğinizi aldık. Randevunuz işletmenin iptal kuralına göre iptal edilecek ve size ayrıca bilgi verilecek. İyi günler.",
	3: "Başka bir saat isteğinizi işletmeye iletiyoruz. İşletme yeni saati onaylayana kadar mevcut randevunuz geçerlidir. İyi günler.",
}

// Accounts reads integration accounts (the integration accounts service).
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

// Inbound runs a provider notification once (the integration job runner).
type Inbound interface {
	HandleInbound(ctx context.Context, in jobsusecase.InboundInput, fn jobsusecase.InboundHandler) (jobsusecase.InboundResult, error)
}

// Answerer applies a key to its visit (the customer panel).
type Answerer interface {
	ChannelAnswer(ctx context.Context, orgID, reservationID int64, answer, source string) (customerpanelusecase.Changed, error)
}

// Requests is the confirmation engine: whether a request is the booking's
// current one, and the call made again after a missed one.
type Requests interface {
	RequestCurrent(ctx context.Context, d db.CustomerNotificationDispatch) (bool, error)
	CallMissed(ctx context.Context, d db.CustomerNotificationDispatch, at time.Time) error
}

// Recorder records delivery reports (the customer notification engine).
type Recorder interface {
	RecordDelivery(ctx context.Context, r channel.Report) (int64, bool, error)
}

// Client orders calls (netgsm.Provider).
type Client interface {
	Send(ctx context.Context, account integrations.Account, c netgsm.Call) (string, error)
}

// Signer makes the keyed hash of the webhook tokens (crypto.FieldCipher).
type Signer interface {
	BlindIndex(purpose, scope, normalized string) string
}

// Service is the voice channel and webhook.
type Service struct {
	q           *db.Queries
	client      Client
	accounts    Accounts
	inbound     Inbound
	reports     Recorder
	answers     Answerer
	requests    Requests
	signer      Signer
	webhookBase string
	events      resourceevents.Publisher
	activity    *activity.Recorder
	log         *slog.Logger
	now         func() time.Time
}

// New builds the service. client nil = the provider is not enabled in this
// process (the channel is not ready, the webhook knows no account).
// webhookBase is the public API origin the provider posts results to
// ("https://app.example/api").
func New(q *db.Queries, client Client, webhookBase string) *Service {
	if p, ok := client.(*netgsm.Provider); ok && p == nil {
		client = nil
	}
	return &Service{q: q, client: client, webhookBase: strings.TrimRight(strings.TrimSpace(webhookBase), "/"),
		log: slog.Default(), now: time.Now}
}

// SetAccounts plugs in the integration accounts, the inbound runner and the
// token signer (the integration kit is built after the notification
// engine).
func (s *Service) SetAccounts(a Accounts, in Inbound, signer Signer) {
	s.accounts, s.inbound, s.signer = a, in, signer
}

// SetAnswers plugs in the customer panel (answers) and the confirmation
// engine (current request, calls made again).
func (s *Service) SetAnswers(a Answerer, r Requests) { s.answers, s.requests = a, r }

// SetResourceEvents publishes applied answers on the reservations live
// channel.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) { s.events = p }

// SetActivity records applied answers in the business's activity history.
func (s *Service) SetActivity(r *activity.Recorder) { s.activity = r }

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// enabled reports whether calls can be made.
func (s *Service) enabled() bool {
	return s.client != nil && s.accounts != nil && s.signer != nil && s.webhookBase != ""
}

// token is the webhook token of one attempt of one account.
func (s *Service) token(account, delivery uuid.UUID) string {
	return s.signer.BlindIndex(WebhookPurpose, account.String(), delivery.String())
}

// WebhookURL is the address the provider posts an attempt's result to.
func (s *Service) WebhookURL(account, delivery uuid.UUID) string {
	return s.webhookBase + "/v1/public/voice/netgsm/" + account.String() + "/webhook?d=" + delivery.String() +
		"&s=" + s.token(account, delivery)
}

// webhookAccount resolves the account of a webhook URL.
func (s *Service) webhookAccount(ctx context.Context, id uuid.UUID) (integrations.Account, error) {
	if !s.enabled() {
		return integrations.Account{}, ErrUnknownAccount
	}
	row, err := s.q.GetWebhookIntegrationAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return integrations.Account{}, ErrUnknownAccount
	}
	if err != nil {
		return integrations.Account{}, err
	}
	// A disabled account still takes the results of calls it made.
	if row.Kind != integrations.KindVoiceCall || row.Provider != netgsm.ProviderCode {
		return integrations.Account{}, ErrUnknownAccount
	}
	acc, err := s.accounts.AccountByID(ctx, row.OrganizationID, row.ID)
	if err != nil {
		return integrations.Account{}, ErrUnknownAccount
	}
	return acc, nil
}

// HandleWebhook verifies and applies one result post of an account: the
// attempt (delivery) and its token come from the address we gave with the
// call.
func (s *Service) HandleWebhook(ctx context.Context, id uuid.UUID, delivery, token string, body []byte) error {
	acc, err := s.webhookAccount(ctx, id)
	if err != nil {
		return err
	}
	dlID, err := uuid.Parse(delivery)
	if err != nil || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.token(acc.UUID, dlID))) != 1 {
		s.log.WarnContext(ctx, "voice_webhook_rejected", "organization", acc.OrganizationID, "account", acc.UUID)
		return ErrSignature
	}
	results, err := netgsm.ParseResult(body)
	if err != nil {
		return ErrMalformed
	}
	dl, err := s.q.GetCustomerNotificationDeliveryByUUID(ctx, db.GetCustomerNotificationDeliveryByUUIDParams{
		OrganizationID: acc.OrganizationID, Uuid: dlID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && dl.Channel != channel.VoiceCall) {
		return nil
	}
	if err != nil {
		return err
	}
	var failed error
	for _, r := range results {
		// Another order's result (the attempt names its own order id).
		if dl.ProviderRef != "" && r.BulkID != dl.ProviderRef {
			continue
		}
		if err := s.result(ctx, acc, dl, r); err != nil {
			s.log.WarnContext(ctx, "voice_result_failed", "organization", acc.OrganizationID, "order", r.BulkID, "error", err)
			failed = err
		}
	}
	return failed
}

// result handles one call result once.
func (s *Service) result(ctx context.Context, acc integrations.Account, dl db.CustomerNotificationDelivery, r netgsm.Result) error {
	ref := dl.Uuid
	_, err := s.inbound.HandleInbound(ctx, jobsusecase.InboundInput{
		OrganizationID: acc.OrganizationID, AccountID: acc.ID, Provider: netgsm.ProviderCode, Kind: InboundKind,
		Key:     "netgsm:call:" + r.BulkID + ":" + dl.Uuid.String(),
		Payload: map[string]any{"delivery_uuid": dl.Uuid.String(), "state": r.State, "key": r.Key},
		RefType: "notification_delivery", RefUUID: &ref,
	}, func(ctx context.Context) (string, map[string]any, error) {
		return s.apply(ctx, acc, dl, r)
	})
	return err
}

// outcome reads a call result: the report for the attempt, the answer of
// the key ("" = no key) and whether the call counts as missed (made again
// when the rule allows).
func outcome(r netgsm.Result) (rep channel.Report, answer string, missed bool) {
	at := r.HungUpAt
	if at.IsZero() {
		at = r.AnsweredAt
	}
	rep = channel.Report{Channel: channel.VoiceCall, At: at, ProviderRef: r.BulkID}
	switch r.State {
	case netgsm.StateAnswered:
		if !r.AnsweredAt.IsZero() {
			rep.At = r.AnsweredAt
		}
		if a, ok := KeyAnswers[r.Key]; ok {
			rep.Delivered = true
			return rep, a, false
		}
		rep.Unanswered, rep.Error = true, ErrNoKey
		if r.Key != netgsm.NoKey {
			rep.Error = ErrInvalidKey
			return rep, AnswerUnknown, true
		}
		return rep, "", true
	case netgsm.StateNotAnswered:
		rep.Error = ErrNotAnswered
	case netgsm.StateUnreachable:
		rep.Error = ErrUnreachable
	case netgsm.StateBusy:
		rep.Error = ErrBusy
	default:
		rep.Error = ErrCallFailed
	}
	return rep, "", true
}

// apply records the result as the attempt's delivery report, applies the
// key, and makes a missed call again.
func (s *Service) apply(ctx context.Context, acc integrations.Account, dl db.CustomerNotificationDelivery, r netgsm.Result) (string, map[string]any, error) {
	orgID := acc.OrganizationID
	rep, answer, missed := outcome(r)
	rep.OrganizationID, rep.DeliveryUUID = orgID, &dl.Uuid
	if s.reports != nil {
		// Recorded only: a failed attempt goes back to its dispatch, and the
		// worker sends the next channel (with its links and usage).
		if _, _, err := s.reports.RecordDelivery(ctx, rep); err != nil {
			return "", nil, err
		}
	}
	d, err := s.q.GetCustomerNotificationDispatch(ctx, db.GetCustomerNotificationDispatchParams{OrganizationID: orgID, ID: dl.DispatchID})
	if err != nil {
		return "", nil, err
	}
	summary := map[string]any{"state": r.State, "key": r.Key, "missed": missed}
	if answer != "" {
		result, reason, changed, err := s.decide(ctx, d, r, answer)
		if err != nil {
			return "", nil, err
		}
		at := r.AnsweredAt
		if at.IsZero() {
			at = s.now()
		}
		if _, err := s.q.InsertCustomerNotificationResponse(ctx, db.InsertCustomerNotificationResponseParams{
			OrganizationID: orgID, DeliveryID: dl.ID, Channel: channel.VoiceCall, Provider: netgsm.ProviderCode,
			ProviderRef: truncate(r.BulkID, 128), Answer: answer, Result: result, Reason: truncate(reason, 255),
			RespondedAt: pgtype.Timestamptz{Time: at.UTC(), Valid: true},
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", nil, err
		}
		if changed != nil {
			s.record(ctx, answer, *changed)
		}
		summary["answer"], summary["result"], summary["reason"] = answer, result, reason
		if result == ResultApplied {
			return jobsusecase.StatusSucceeded, summary, nil
		}
	}
	if missed && s.requests != nil {
		if err := s.requests.CallMissed(ctx, d, rep.At); err != nil {
			return "", nil, err
		}
	}
	return jobsusecase.StatusIgnored, summary, nil
}

// decide checks and applies the answer of a key.
func (s *Service) decide(ctx context.Context, d db.CustomerNotificationDispatch, r netgsm.Result, answer string) (string, string, *customerpanelusecase.Changed, error) {
	orgID := d.OrganizationID
	if d.EventCode != catalog.ConfirmationRequest || !d.ReservationID.Valid {
		return ResultIgnored, ReasonNotConfirmation, nil, nil
	}
	if answer == AnswerUnknown {
		return ResultIgnored, ReasonInvalidKey, nil, nil
	}
	cust, err := s.q.GetNotifyCustomer(ctx, db.GetNotifyCustomerParams{OrganizationID: orgID, ID: d.CustomerID})
	if err != nil {
		return "", "", nil, err
	}
	// The provider reports the number it called; it must still be the
	// customer's.
	if r.Callee != "" && netgsm.Recipient(r.Callee) != netgsm.Recipient(cust.PhoneE164.String) {
		return ResultIgnored, ReasonOtherNumber, nil, nil
	}
	current := false
	if s.requests != nil {
		if current, err = s.requests.RequestCurrent(ctx, d); err != nil {
			return "", "", nil, err
		}
	}
	if !current {
		return ResultRejected, ReasonOutdated, nil, nil
	}
	if s.answers == nil {
		return ResultRejected, ReasonNotOpen, nil, nil
	}
	out, err := s.answers.ChannelAnswer(ctx, orgID, d.ReservationID.Int64, answer, confirmationsmodel.SourceVoiceCall)
	var ae *customerpanelusecase.ActionError
	switch {
	case err == nil:
		return ResultApplied, "", &out, nil
	case errors.As(err, &ae):
		return ResultRejected, ae.Reason, nil, nil
	case errors.Is(err, customerpanelusecase.ErrNotFound):
		return ResultRejected, ReasonNotFound, nil, nil
	case errors.Is(err, reservationsusecase.ErrInvalidTransition), errors.Is(err, reservationsusecase.ErrStaleVersion):
		return ResultRejected, ReasonNotOpen, nil, nil
	}
	return "", "", nil, err
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// record writes the activity of an applied answer and tells the live lists.
func (s *Service) record(ctx context.Context, answer string, c customerpanelusecase.Changed) {
	ctx = orgctx.WithScope(ctx, c.Scope)
	if s.events != nil {
		s.events.Publish(ctx, reservationsmodel.Slug, resourceevents.ActionUpdated, c.Reservation.UUID.String())
	}
	if s.activity == nil {
		return
	}
	action, payload := "reservations.confirmed", map[string]any{}
	switch answer {
	case confirmationsmodel.AnswerChange:
		action = "reservations.change_requested"
	case confirmationsmodel.AnswerCancel:
		action, payload["reason"] = "reservations.cancelled", customerpanelusecase.CancelReason
	}
	payload["number"], payload["source"] = c.Reservation.Number, confirmationsmodel.SourceVoiceCall
	id, branch := c.Reservation.UUID, c.Reservation.Branch.UUID
	s.activity.RecordEntry(ctx, activity.Entry{
		Action: action, Resource: reservationsmodel.Slug, ResourceUUID: &id, BranchUUID: &branch, Payload: payload,
	})
}

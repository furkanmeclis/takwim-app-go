// Package usecase is the WhatsApp channel of the confirmation requests
// (TEC-922, docs/design/confirmations.md "Channels"): the customer
// notification channel that sends a confirmation request as the business's
// approved template with Geleceğim / İptal / Değişiklik buttons (Channel),
// and the webhook of the business's account (Service.HandleWebhook) that
// turns delivery statuses into delivery reports and button replies into
// answers.
//
//   - Acceptance, delivery, read and the reply are recorded apart
//     (customer_notification_deliveries.sent_at / delivered_at / read_at,
//     customer_notification_responses). A delivered or read message is
//     never a confirmation; only a button reply is an answer.
//   - A reply acts only when it answers our message (context id), comes
//     from the customer's own number and belongs to the booking's current
//     request (an older request of a moved visit never acts). Anything else
//     is recorded as ignored or rejected, never applied silently.
//   - Each inbound message is handled once (integration inbound jobs keyed
//     by the provider's message id); an internal failure answers 500 so the
//     provider delivers it again.
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	confirmationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	customerpanelmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	customerpanelusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/meta"
)

// InboundKind is the integration job kind of a webhook reply.
const InboundKind = "whatsapp.reply"

// PayloadPrefix starts the quick reply payloads of the confirmation
// template ("tkw1:confirm").
const PayloadPrefix = "tkw1:"

// Answers of a reply (customer_notification_responses.answer).
const (
	AnswerUnknown = "unknown"
)

// Results of a reply.
const (
	ResultApplied  = "applied"
	ResultRejected = "rejected"
	ResultIgnored  = "ignored"
)

// Reasons of a reply that was not applied.
const (
	ReasonNotConfirmation = "not_confirmation_request"
	ReasonOtherNumber     = "other_number"
	ReasonNotAnswer       = "not_an_answer"
	ReasonOutdated        = "request_outdated"
	ReasonNotFound        = "reservation_not_found"
	ReasonNotOpen         = "not_open"
)

var (
	// ErrUnknownAccount: the webhook names no WhatsApp account of ours.
	ErrUnknownAccount = errors.New("unknown WhatsApp account")
	// ErrSignature: the webhook is not signed with the account's secret.
	ErrSignature = errors.New("invalid webhook signature")
	// ErrMalformed: the body is not a WhatsApp webhook.
	ErrMalformed = errors.New("malformed webhook")
)

// Accounts reads integration accounts (the integration accounts service).
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

// Inbound runs a provider notification once (the integration job runner).
type Inbound interface {
	HandleInbound(ctx context.Context, in jobsusecase.InboundInput, fn jobsusecase.InboundHandler) (jobsusecase.InboundResult, error)
}

// Answerer applies a reply to its visit (the customer panel).
type Answerer interface {
	ChannelAnswer(ctx context.Context, orgID, reservationID int64, answer, source string) (customerpanelusecase.Changed, error)
}

// Requests tells whether a request is the booking's current one (the
// confirmation engine).
type Requests interface {
	RequestCurrent(ctx context.Context, d db.CustomerNotificationDispatch) (bool, error)
}

// Recorder records delivery reports (the customer notification engine).
type Recorder interface {
	RecordDelivery(ctx context.Context, r channel.Report) (int64, bool, error)
}

// Client is the Cloud API (meta.Provider).
type Client interface {
	SendTemplate(ctx context.Context, account integrations.Account, m meta.TemplateMessage) (string, error)
	SendText(ctx context.Context, account integrations.Account, to, body string) (string, error)
}

// Service is the WhatsApp channel and webhook.
type Service struct {
	q        *db.Queries
	client   Client
	accounts Accounts
	inbound  Inbound
	reports  Recorder
	answers  Answerer
	requests Requests
	events   resourceevents.Publisher
	activity *activity.Recorder
	log      *slog.Logger
}

// New builds the service. client nil = the Cloud API is not enabled in
// this process (the channel is not ready, the webhook knows no account).
func New(q *db.Queries, client Client) *Service {
	if p, ok := client.(*meta.Provider); ok && p == nil {
		client = nil
	}
	return &Service{q: q, client: client, log: slog.Default()}
}

// SetAccounts plugs in the integration accounts and the inbound runner (the
// integration kit is built after the notification engine).
func (s *Service) SetAccounts(a Accounts, in Inbound) { s.accounts, s.inbound = a, in }

// SetAnswers plugs in the customer panel (answers) and the confirmation
// engine (current request).
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

// enabled reports whether the Cloud API can be used.
func (s *Service) enabled() bool { return s.client != nil && s.accounts != nil }

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
	if row.Kind != integrations.KindWhatsApp || row.Provider != meta.ProviderCode || row.Status != "active" {
		return integrations.Account{}, ErrUnknownAccount
	}
	acc, err := s.accounts.AccountByID(ctx, row.OrganizationID, row.ID)
	if err != nil {
		return integrations.Account{}, ErrUnknownAccount
	}
	return acc, nil
}

// Challenge answers the subscription handshake of a webhook URL: the
// challenge to echo, or ErrSignature.
func (s *Service) Challenge(ctx context.Context, id uuid.UUID, mode, token, challenge string) (string, error) {
	acc, err := s.webhookAccount(ctx, id)
	if err != nil {
		return "", err
	}
	if !meta.VerifyChallenge(acc, mode, token) || challenge == "" {
		return "", ErrSignature
	}
	return challenge, nil
}

// HandleWebhook verifies and applies one webhook call of an account.
func (s *Service) HandleWebhook(ctx context.Context, id uuid.UUID, header http.Header, body []byte) error {
	acc, err := s.webhookAccount(ctx, id)
	if err != nil {
		return err
	}
	if !meta.VerifySignature(acc, header.Get("X-Hub-Signature-256"), body) {
		s.log.WarnContext(ctx, "whatsapp_webhook_rejected", "organization", acc.OrganizationID, "account", acc.UUID)
		return ErrSignature
	}
	w, err := meta.ParseWebhook(body)
	if err != nil {
		return ErrMalformed
	}
	number := acc.Credentials[meta.FieldPhoneNumberID]
	var failed error
	for _, st := range w.Statuses {
		if st.PhoneNumberID != "" && st.PhoneNumberID != number {
			continue
		}
		if err := s.status(ctx, acc, st); err != nil {
			failed = err
		}
	}
	for _, r := range w.Replies {
		if r.PhoneNumberID != "" && r.PhoneNumberID != number {
			continue
		}
		if err := s.reply(ctx, acc, r); err != nil {
			s.log.WarnContext(ctx, "whatsapp_reply_failed", "organization", acc.OrganizationID, "message", r.MessageID, "error", err)
			failed = err
		}
	}
	return failed
}

func (s *Service) delivery(ctx context.Context, orgID int64, ref string) (db.CustomerNotificationDelivery, bool, error) {
	if ref == "" {
		return db.CustomerNotificationDelivery{}, false, nil
	}
	dl, err := s.q.GetCustomerNotificationDeliveryByProviderRef(ctx, db.GetCustomerNotificationDeliveryByProviderRefParams{
		OrganizationID: orgID, Channel: channel.WhatsApp, ProviderRef: ref,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dl, false, nil
	}
	return dl, err == nil, err
}

// status turns a delivery status into the engine's delivery report. "sent"
// only repeats the acceptance; "delivered" and "read" are receipts; a
// failure runs the rule's next channel.
func (s *Service) status(ctx context.Context, acc integrations.Account, st meta.Status) error {
	dl, ok, err := s.delivery(ctx, acc.OrganizationID, st.MessageID)
	if err != nil || !ok || s.reports == nil {
		return err
	}
	rep := channel.Report{Channel: channel.WhatsApp, OrganizationID: acc.OrganizationID, DeliveryUUID: &dl.Uuid, At: st.At}
	switch st.Status {
	case meta.StatusDelivered:
		rep.Delivered = true
	case meta.StatusRead:
		rep.Delivered, rep.Read = true, true
	case meta.StatusFailed:
		rep.Error = st.Error
		if rep.Error == "" {
			rep.Error = "not delivered"
		}
	default:
		return nil
	}
	// Recorded only: a failed attempt goes back to its dispatch, and the
	// worker sends the next channel (with its links and usage).
	_, _, err = s.reports.RecordDelivery(ctx, rep)
	return err
}

// answerOf reads the answer of a quick reply payload.
func answerOf(payload string) string {
	v, ok := strings.CutPrefix(strings.TrimSpace(payload), PayloadPrefix)
	if !ok {
		return AnswerUnknown
	}
	switch v {
	case confirmationsmodel.AnswerConfirm, confirmationsmodel.AnswerCancel, confirmationsmodel.AnswerChange:
		return v
	}
	return AnswerUnknown
}

// reply handles one inbound message once.
func (s *Service) reply(ctx context.Context, acc integrations.Account, r meta.Reply) error {
	if r.MessageID == "" {
		return nil
	}
	// Only answers to our own messages concern us; other chats of the
	// business's number are left alone.
	dl, ok, err := s.delivery(ctx, acc.OrganizationID, r.ContextID)
	if err != nil || !ok {
		return err
	}
	answer := answerOf(r.Payload)
	ref := dl.Uuid
	_, err = s.inbound.HandleInbound(ctx, jobsusecase.InboundInput{
		OrganizationID: acc.OrganizationID, AccountID: acc.ID, Provider: meta.ProviderCode, Kind: InboundKind,
		Key:     "whatsapp:reply:" + r.MessageID,
		Payload: map[string]any{"delivery_uuid": dl.Uuid.String(), "answer": answer, "type": r.Type},
		RefType: "notification_delivery", RefUUID: &ref,
	}, func(ctx context.Context) (string, map[string]any, error) {
		return s.applyReply(ctx, acc, dl, r, answer)
	})
	return err
}

func digits(v string) string { return meta.Recipient(v) }

// applyReply decides and applies a reply, records it and acknowledges it
// to the customer.
func (s *Service) applyReply(ctx context.Context, acc integrations.Account, dl db.CustomerNotificationDelivery, r meta.Reply, answer string) (string, map[string]any, error) {
	orgID := acc.OrganizationID
	result, reason := ResultIgnored, ""
	var changed *customerpanelusecase.Changed
	d, err := s.q.GetCustomerNotificationDispatch(ctx, db.GetCustomerNotificationDispatchParams{OrganizationID: orgID, ID: dl.DispatchID})
	if err != nil {
		return "", nil, err
	}
	switch {
	case d.EventCode != catalog.ConfirmationRequest || !d.ReservationID.Valid:
		reason = ReasonNotConfirmation
	case answer == AnswerUnknown:
		reason = ReasonNotAnswer
	default:
		cust, err := s.q.GetNotifyCustomer(ctx, db.GetNotifyCustomerParams{OrganizationID: orgID, ID: d.CustomerID})
		if err != nil {
			return "", nil, err
		}
		if digits(cust.PhoneE164.String) == "" || digits(cust.PhoneE164.String) != digits(r.From) {
			reason = ReasonOtherNumber
			break
		}
		current := false
		if s.requests != nil {
			if current, err = s.requests.RequestCurrent(ctx, d); err != nil {
				return "", nil, err
			}
		}
		if !current {
			result, reason = ResultRejected, ReasonOutdated
			break
		}
		if s.answers == nil {
			result, reason = ResultRejected, ReasonNotOpen
			break
		}
		out, err := s.answers.ChannelAnswer(ctx, orgID, d.ReservationID.Int64, answer, confirmationsmodel.SourceWhatsApp)
		var ae *customerpanelusecase.ActionError
		switch {
		case err == nil:
			result, changed = ResultApplied, &out
		case errors.As(err, &ae):
			result, reason = ResultRejected, ae.Reason
		case errors.Is(err, customerpanelusecase.ErrNotFound):
			result, reason = ResultRejected, ReasonNotFound
		case errors.Is(err, reservationsusecase.ErrInvalidTransition), errors.Is(err, reservationsusecase.ErrStaleVersion):
			result, reason = ResultRejected, ReasonNotOpen
		default:
			return "", nil, err
		}
	}
	at := r.At
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := s.q.InsertCustomerNotificationResponse(ctx, db.InsertCustomerNotificationResponseParams{
		OrganizationID: orgID, DeliveryID: dl.ID, Channel: channel.WhatsApp, Provider: meta.ProviderCode,
		ProviderRef: truncate(r.MessageID, 128), Answer: answer, Result: result, Reason: truncate(reason, 255),
		RespondedAt: pgtype.Timestamptz{Time: at.UTC(), Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, err
	}
	if changed != nil {
		s.record(ctx, answer, *changed)
	}
	if reason != ReasonOtherNumber && reason != ReasonNotConfirmation {
		s.acknowledge(ctx, acc, r.From, answer, result, reason)
	}
	summary := map[string]any{"answer": answer, "result": result, "reason": reason}
	if result == ResultApplied {
		return jobsusecase.StatusSucceeded, summary, nil
	}
	return jobsusecase.StatusIgnored, summary, nil
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
	payload["number"], payload["source"] = c.Reservation.Number, confirmationsmodel.SourceWhatsApp
	id, branch := c.Reservation.UUID, c.Reservation.Branch.UUID
	s.activity.RecordEntry(ctx, activity.Entry{
		Action: action, Resource: reservationsmodel.Slug, ResourceUUID: &id, BranchUUID: &branch, Payload: payload,
	})
}

// acknowledge answers the customer in the chat (inside the service window
// a free text is allowed). It is best effort: the answer is recorded either
// way.
func (s *Service) acknowledge(ctx context.Context, acc integrations.Account, to, answer, result, reason string) {
	if s.client == nil || to == "" {
		return
	}
	text := ackText(answer, result, reason)
	if _, err := s.client.SendText(ctx, acc, to, text); err != nil {
		s.log.WarnContext(ctx, "whatsapp_ack_failed", "organization", acc.OrganizationID, "error", err)
	}
}

// ackText is the chat answer to a reply (Turkish: the customer's language
// of the template).
func ackText(answer, result, reason string) string {
	if result == ResultApplied {
		switch answer {
		case confirmationsmodel.AnswerConfirm:
			return "Teşekkürler, randevunuz teyit edildi. Görüşmek üzere!"
		case confirmationsmodel.AnswerCancel:
			return "Randevunuz iptal edildi. Yeni bir randevu almak isterseniz işletmeyle iletişime geçebilirsiniz."
		case confirmationsmodel.AnswerChange:
			return "Değişiklik isteğiniz işletmeye iletildi. Randevunuz işletme yeni saati onaylayana kadar geçerli kalır."
		}
	}
	switch reason {
	case ReasonNotAnswer:
		return "Yanıtınızı anlayamadık. Lütfen mesajdaki düğmelerden birini seçin."
	case ReasonOutdated:
		return "Bu mesaj randevunuzun eski bir bilgisine ait. Lütfen son gönderilen mesajı kullanın."
	case customerpanelmodel.ReasonAnswered:
		return "Bu randevu için yanıtınız daha önce alınmıştı."
	case customerpanelmodel.ReasonStatus, ReasonNotOpen, ReasonNotFound:
		return "Bu randevu artık yanıt beklemiyor. Bilgi için lütfen işletmeyle iletişime geçin."
	}
	if answer == confirmationsmodel.AnswerCancel {
		return "Randevunuz bu kanaldan iptal edilemedi (iptal süresi geçmiş olabilir). Lütfen işletmeyle iletişime geçin."
	}
	return "Yanıtınız bu randevuya uygulanamadı. Lütfen işletmeyle iletişime geçin."
}

// Package meta is the WhatsApp Cloud API adapter (TEC-922, requirements
// §16 R1): a business connects its own WhatsApp Business number on Meta
// (phone number id, a system user access token, the Meta app secret and a
// webhook verify token, sealed by the integration vault). Takwim sends the
// confirmation request as the business's approved template with three
// quick reply buttons (Geleceğim / İptal / Değişiklik) and reads the
// delivery statuses and the button replies from the webhook of the
// account (/api/v1/public/whatsapp/meta/{account}/webhook), signed with the
// app secret (X-Hub-Signature-256).
//
// The adapter is registered only when WHATSAPP_META_ENABLED is true; the
// provider account and the template approval are the business's
// (docs/design/confirmations.md, "Channels").
package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// ProviderCode is the provider code of the WhatsApp Cloud API.
const ProviderCode = "meta"

// Defaults.
const (
	DefaultBaseURL    = "https://graph.facebook.com"
	DefaultAPIVersion = "v21.0"
	// DefaultTemplate / DefaultLanguage name the confirmation template when
	// the account does not (docs/design/confirmations.md has its text).
	DefaultTemplate = "randevu_teyit"
	DefaultLanguage = "tr"
)

// Credential fields.
const (
	FieldPhoneNumberID = "phone_number_id"
	FieldAccessToken   = "access_token"
	FieldAppSecret     = "app_secret"
	FieldVerifyToken   = "verify_token"
	FieldTemplate      = "template_name"
	FieldLanguage      = "template_language"
)

// Provider is the Cloud API client and the integrations adapter.
type Provider struct {
	BaseURL    string
	APIVersion string
	Client     *http.Client
}

// New builds the adapter (empty values take the defaults).
func New(baseURL, version string, client *http.Client) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if version == "" {
		version = DefaultAPIVersion
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Provider{BaseURL: strings.TrimRight(baseURL, "/"), APIVersion: strings.Trim(version, "/"), Client: client}
}

var _ integrations.Adapter = (*Provider)(nil)

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return integrations.KindWhatsApp }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: FieldPhoneNumberID, LabelKey: "integrations.fields.phone_number_id", Required: true},
		{Key: FieldAccessToken, LabelKey: "integrations.fields.access_token", Secret: true, Required: true},
		{Key: FieldAppSecret, LabelKey: "integrations.fields.app_secret", Secret: true, Required: true},
		{Key: FieldVerifyToken, LabelKey: "integrations.fields.verify_token", Secret: true, Required: true},
		{Key: FieldTemplate, LabelKey: "integrations.fields.template_name"},
		{Key: FieldLanguage, LabelKey: "integrations.fields.template_language"},
	}
}

var phoneNumberIDRE = regexp.MustCompile(`^[0-9]{5,32}$`)

// Check implements integrations.Adapter: reads the number with the token
// (no message is sent).
func (p *Provider) Check(ctx context.Context, account integrations.Account) error {
	id := account.Credentials[FieldPhoneNumberID]
	if !phoneNumberIDRE.MatchString(id) {
		return errors.New("whatsapp: phone_number_id must be the numeric id of the number")
	}
	if account.Credentials[FieldAppSecret] == "" || account.Credentials[FieldVerifyToken] == "" {
		return errors.New("whatsapp: app_secret and verify_token are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.endpoint(id)+"?fields="+url.QueryEscape("display_phone_number,verified_name"), nil)
	if err != nil {
		return err
	}
	_, err = p.do(req, account)
	return err
}

// Template is the account's confirmation template and its language.
func Template(account integrations.Account) (name, language string) {
	name, language = strings.TrimSpace(account.Credentials[FieldTemplate]), strings.TrimSpace(account.Credentials[FieldLanguage])
	if name == "" {
		name = DefaultTemplate
	}
	if language == "" {
		language = DefaultLanguage
	}
	return name, language
}

// TemplateMessage is an approved template with body parameters and quick
// reply payloads (one per button, in the template's order).
type TemplateMessage struct {
	To           string
	Name         string
	Language     string
	BodyParams   []string
	QuickReplies []string
}

// Error is an error answer of the Graph API.
type Error struct {
	Status  int
	Code    int
	Message string
}

func (e *Error) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("whatsapp: %d %s", e.Code, e.Message)
	}
	return fmt.Sprintf("whatsapp: http %d %s", e.Status, e.Message)
}

// Recipient turns an E.164 number into the Cloud API's form (digits only).
func Recipient(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SendTemplate sends a template message and answers the message id.
func (p *Provider) SendTemplate(ctx context.Context, account integrations.Account, m TemplateMessage) (string, error) {
	params := make([]map[string]any, 0, len(m.BodyParams))
	for _, v := range m.BodyParams {
		if strings.TrimSpace(v) == "" {
			v = "-" // the Cloud API refuses an empty parameter
		}
		params = append(params, map[string]any{"type": "text", "text": v})
	}
	components := []map[string]any{}
	if len(params) > 0 {
		components = append(components, map[string]any{"type": "body", "parameters": params})
	}
	for i, payload := range m.QuickReplies {
		components = append(components, map[string]any{
			"type": "button", "sub_type": "quick_reply", "index": strconv.Itoa(i),
			"parameters": []map[string]any{{"type": "payload", "payload": payload}},
		})
	}
	return p.send(ctx, account, map[string]any{
		"messaging_product": "whatsapp", "recipient_type": "individual", "to": Recipient(m.To), "type": "template",
		"template": map[string]any{"name": m.Name, "language": map[string]any{"code": m.Language}, "components": components},
	})
}

// SendText sends a free text message (only inside the customer service
// window, i.e. right after the customer wrote).
func (p *Provider) SendText(ctx context.Context, account integrations.Account, to, body string) (string, error) {
	return p.send(ctx, account, map[string]any{
		"messaging_product": "whatsapp", "recipient_type": "individual", "to": Recipient(to), "type": "text",
		"text": map[string]any{"body": body, "preview_url": false},
	})
}

func (p *Provider) endpoint(id string) string {
	return p.BaseURL + "/" + p.APIVersion + "/" + url.PathEscape(id)
}

func (p *Provider) send(ctx context.Context, account integrations.Account, body map[string]any) (string, error) {
	id := account.Credentials[FieldPhoneNumberID]
	if !phoneNumberIDRE.MatchString(id) {
		return "", &Error{Message: "phone_number_id is not set"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(id)+"/messages", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.do(req, account)
	if err != nil {
		return "", err
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(resp, &out); err != nil || len(out.Messages) == 0 || out.Messages[0].ID == "" {
		return "", &Error{Status: http.StatusOK, Message: "answer without a message id"}
	}
	return out.Messages[0].ID, nil
}

func (p *Provider) do(req *http.Request, account integrations.Account) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+account.Credentials[FieldAccessToken])
	res, err := p.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("whatsapp: read answer: %w", err)
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return body, nil
	}
	var e struct {
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	msg := e.Error.Message
	if msg == "" {
		msg = http.StatusText(res.StatusCode)
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return nil, &Error{Status: res.StatusCode, Code: e.Error.Code, Message: msg}
}

// VerifySignature checks X-Hub-Signature-256 ("sha256=<hex>") over the raw
// body with the account's app secret.
func VerifySignature(account integrations.Account, header string, body []byte) bool {
	secret := account.Credentials[FieldAppSecret]
	sig, ok := strings.CutPrefix(strings.TrimSpace(header), "sha256=")
	if secret == "" || !ok {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

// Sign is the X-Hub-Signature-256 value of body (tests, fakes).
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyChallenge checks the webhook subscription handshake
// (hub.mode=subscribe, hub.verify_token).
func VerifyChallenge(account integrations.Account, mode, token string) bool {
	want := account.Credentials[FieldVerifyToken]
	return mode == "subscribe" && want != "" && subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1
}

// Status values of a delivery report.
const (
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
)

// Status is one delivery report of a message we sent.
type Status struct {
	PhoneNumberID string
	MessageID     string
	Status        string
	Recipient     string
	At            time.Time
	Error         string
}

// Reply is one inbound message: a button press (Payload) or a text.
type Reply struct {
	PhoneNumberID string
	MessageID     string
	From          string
	// ContextID is the message the customer answered (ours).
	ContextID string
	Type      string
	Payload   string
	Text      string
	At        time.Time
}

// Webhook is the content of one webhook call.
type Webhook struct {
	Statuses []Status
	Replies  []Reply
}

type wireWebhook struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Statuses []struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					Timestamp   string `json:"timestamp"`
					RecipientID string `json:"recipient_id"`
					Errors      []struct {
						Code    int    `json:"code"`
						Title   string `json:"title"`
						Message string `json:"message"`
					} `json:"errors"`
				} `json:"statuses"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Context   struct {
						ID string `json:"id"`
					} `json:"context"`
					Button struct {
						Payload string `json:"payload"`
						Text    string `json:"text"`
					} `json:"button"`
					Interactive struct {
						ButtonReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply"`
					} `json:"interactive"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func unix(v string) time.Time {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

// ErrMalformed: the body is not a WhatsApp webhook.
var ErrMalformed = errors.New("whatsapp: malformed webhook")

// ParseWebhook reads the statuses and replies of a webhook body.
func ParseWebhook(body []byte) (Webhook, error) {
	var w wireWebhook
	if err := json.Unmarshal(body, &w); err != nil || w.Object != "whatsapp_business_account" {
		return Webhook{}, ErrMalformed
	}
	out := Webhook{}
	for _, e := range w.Entry {
		for _, c := range e.Changes {
			if c.Field != "messages" {
				continue
			}
			v := c.Value
			for _, s := range v.Statuses {
				st := Status{
					PhoneNumberID: v.Metadata.PhoneNumberID, MessageID: s.ID, Status: s.Status, Recipient: s.RecipientID,
					At: unix(s.Timestamp),
				}
				if len(s.Errors) > 0 {
					msg := s.Errors[0].Title
					if s.Errors[0].Message != "" {
						msg = s.Errors[0].Message
					}
					st.Error = strings.TrimSpace(strconv.Itoa(s.Errors[0].Code) + " " + msg)
				}
				out.Statuses = append(out.Statuses, st)
			}
			for _, m := range v.Messages {
				r := Reply{
					PhoneNumberID: v.Metadata.PhoneNumberID, MessageID: m.ID, From: m.From, ContextID: m.Context.ID,
					Type: m.Type, At: unix(m.Timestamp),
				}
				switch m.Type {
				case "button":
					r.Payload, r.Text = m.Button.Payload, m.Button.Text
				case "interactive":
					r.Payload, r.Text = m.Interactive.ButtonReply.ID, m.Interactive.ButtonReply.Title
				case "text":
					r.Text = m.Text.Body
				}
				out.Replies = append(out.Replies, r)
			}
		}
	}
	return out, nil
}

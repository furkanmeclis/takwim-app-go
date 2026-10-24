// Package netgsm is the Netgsm voice message adapter (TEC-923,
// requirements §16 R2): a business connects its own Netgsm account (user
// code and API password, sealed by the integration vault). Takwim calls the
// customer with a text read out by the provider, takes one key (the
// confirmation call: 1, 2 or 3; each key answers with a short text) and
// gets the result of the call (answered, not answered, busy, the key
// pressed) on the webhook address given with the call.
//
// Netgsm signs nothing, so the address carries a token of the call that
// only Takwim can make (the voice module); the adapter only builds the
// request and reads the result.
//
// The adapter is registered only when VOICE_NETGSM_ENABLED is true.
package netgsm

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// ProviderCode is the provider code of Netgsm.
const ProviderCode = "netgsm"

// DefaultBaseURL is Netgsm's API origin.
const DefaultBaseURL = "https://api.netgsm.com.tr"

// Credential fields.
const (
	FieldUserCode = "usercode"
	FieldPassword = "password"
)

// MinWindow is the shortest time Netgsm accepts between the start and the
// end of a call order.
const MinWindow = time.Hour

// Location is the time zone Netgsm reads dates in.
var Location = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// Provider is the Netgsm voice client and the integrations adapter.
type Provider struct {
	BaseURL string
	Client  *http.Client
}

// New builds the adapter (empty values take the defaults).
func New(baseURL string, client *http.Client) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Provider{BaseURL: strings.TrimRight(baseURL, "/"), Client: client}
}

var _ integrations.Adapter = (*Provider)(nil)

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return integrations.KindVoiceCall }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: FieldUserCode, LabelKey: "integrations.fields.usercode", Required: true},
		{Key: FieldPassword, LabelKey: "integrations.fields.password", Secret: true, Required: true},
	}
}

// Error is an error answer of Netgsm (its numeric code).
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return "netgsm: " + e.Code + " " + e.Message
	}
	return "netgsm: " + e.Message
}

// codeMessages are Netgsm's error codes.
var codeMessages = map[string]string{
	"30":  "invalid credentials, no API access or IP not allowed",
	"40":  "order not found",
	"45":  "no voice message permission",
	"60":  "no records",
	"70":  "invalid or missing parameter",
	"80":  "query limit exceeded",
	"100": "system error",
}

func codeError(code string) *Error {
	msg, ok := codeMessages[code]
	if !ok {
		msg = "unexpected answer"
	}
	return &Error{Code: code, Message: msg}
}

// Check implements integrations.Adapter: asks for the report of the last
// hour with the credentials (no call is made). "No records" is a working
// account.
func (p *Provider) Check(ctx context.Context, account integrations.Account) error {
	if account.Credentials[FieldUserCode] == "" || account.Credentials[FieldPassword] == "" {
		return errors.New("netgsm: usercode and password are required")
	}
	now := time.Now().In(Location)
	q := url.Values{}
	q.Set("usercode", account.Credentials[FieldUserCode])
	q.Set("password", account.Credentials[FieldPassword])
	q.Set("type", "2")
	q.Set("bastar", now.Add(-time.Hour).Format("020120061504"))
	q.Set("bittar", now.Format("020120061504"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/voicesms/report/?"+q.Encode(), nil)
	if err != nil {
		return errors.New("netgsm: request could not be built")
	}
	body, err := p.do(req)
	if err != nil {
		return err
	}
	first := strings.TrimSpace(strings.SplitN(strings.ReplaceAll(body, "<br>", "\n"), "\n", 2)[0])
	switch first {
	case "30", "70", "80", "100":
		return codeError(first)
	}
	return nil
}

// Key is one key the customer may press and what the call says back.
type Key struct {
	Digit int
	Text  string
}

// Call is one automatic call.
type Call struct {
	To string
	// Text is read out when the customer picks up.
	Text string
	Keys []Key
	// Start / Stop bound when Netgsm may ring (at least MinWindow apart).
	Start, Stop time.Time
	// RingSeconds is how long the phone rings (10 to 30).
	RingSeconds int
	// URL receives the result of the call.
	URL string
}

// Recipient turns a Turkish number into Netgsm's form (ten digits, no
// country code or leading zero); "" when it is not one.
func Recipient(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "90"):
		d = d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"):
		d = d[1:]
	}
	if len(d) != 10 || d[0] == '0' {
		return ""
	}
	return d
}

func esc(v string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}

// order is the XML body of a call order.
func (p *Provider) order(account integrations.Account, c Call) (string, error) {
	to := Recipient(c.To)
	if to == "" {
		return "", &Error{Message: "not a Turkish phone number"}
	}
	if strings.TrimSpace(c.Text) == "" {
		return "", &Error{Message: "no text to read out"}
	}
	start, stop := c.Start.In(Location), c.Stop.In(Location)
	if stop.Sub(start) < MinWindow {
		stop = start.Add(MinWindow)
	}
	ring := c.RingSeconds
	if ring < 10 || ring > 30 {
		ring = 30
	}
	key := "0"
	if len(c.Keys) > 0 {
		key = "1"
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><mainbody><header>`)
	b.WriteString("<usercode>" + esc(account.Credentials[FieldUserCode]) + "</usercode>")
	b.WriteString("<password>" + esc(account.Credentials[FieldPassword]) + "</password>")
	b.WriteString("<startdate>" + start.Format("02012006") + "</startdate>")
	b.WriteString("<starttime>" + start.Format("1504") + "</starttime>")
	b.WriteString("<stopdate>" + stop.Format("02012006") + "</stopdate>")
	b.WriteString("<stoptime>" + stop.Format("1504") + "</stoptime>")
	b.WriteString("<key>" + key + "</key>")
	b.WriteString("<ringtime>" + strconv.Itoa(ring) + "</ringtime>")
	if c.URL != "" {
		b.WriteString("<url>" + esc(c.URL) + "</url>")
	}
	b.WriteString("</header><body>")
	b.WriteString("<text>" + esc(c.Text) + "</text>")
	b.WriteString("<no>" + to + "</no>")
	for _, k := range c.Keys {
		if k.Digit < 0 || k.Digit > 9 {
			continue
		}
		b.WriteString("<keys><keydetail><keyinfo>" + strconv.Itoa(k.Digit) + "</keyinfo><text>" + esc(k.Text) +
			"</text></keydetail></keys>")
	}
	b.WriteString("</body></mainbody>")
	return b.String(), nil
}

// Send orders a call and answers Netgsm's order id (bulkid).
func (p *Provider) Send(ctx context.Context, account integrations.Account, c Call) (string, error) {
	if account.Credentials[FieldUserCode] == "" || account.Credentials[FieldPassword] == "" {
		return "", &Error{Message: "credentials are not set"}
	}
	body, err := p.order(account, c)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/voicesms/send", strings.NewReader(body))
	if err != nil {
		return "", errors.New("netgsm: request could not be built")
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	out, err := p.do(req)
	if err != nil {
		return "", err
	}
	f := strings.Fields(out)
	if len(f) == 0 {
		return "", &Error{Message: "empty answer"}
	}
	switch f[0] {
	case "00", "01", "02":
		if len(f) < 2 {
			return "", &Error{Code: f[0], Message: "answer without an order id"}
		}
		return f[1], nil
	}
	return "", codeError(f[0])
}

// do runs a request; the error never carries the URL (its query may hold
// the credentials).
func (p *Provider) do(req *http.Request) (string, error) {
	res, err := p.Client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return "", fmt.Errorf("netgsm: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", errors.New("netgsm: answer could not be read")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", &Error{Message: "http " + strconv.Itoa(res.StatusCode)}
	}
	return strings.TrimSpace(string(body)), nil
}

// Call states of a result.
const (
	StateAnswered    = 1
	StateNotAnswered = 2
	StateUnreachable = 3
	StateBusy        = 7
)

// NoKey is Key of a result when the customer pressed nothing.
const NoKey = -1

// Result is the outcome of one call (the webhook).
type Result struct {
	BulkID string
	Callee string
	State  int
	// Key is the digit pressed, or NoKey.
	Key        int
	AnsweredAt time.Time
	HungUpAt   time.Time
}

// flex reads a JSON string, number or null as text.
type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flex(strings.TrimSpace(s))
		return nil
	}
	*f = flex(string(b))
	return nil
}

type wireDetail struct {
	PushButton flex `json:"push_button"`
}

type wireResult struct {
	BulkID     flex            `json:"bulkid"`
	Callee     flex            `json:"callee"`
	State      flex            `json:"state"`
	AnswerTime flex            `json:"answer_time"`
	HangupTime flex            `json:"hangup_time"`
	PushButton flex            `json:"push_button"`
	Detail     json.RawMessage `json:"detail"`
}

// ErrMalformed: the body is not a Netgsm call result.
var ErrMalformed = errors.New("netgsm: malformed call result")

func parseTime(v flex) time.Time {
	s := strings.TrimSpace(string(v))
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, Location); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func keyOf(v flex) int {
	s := strings.TrimSpace(string(v))
	if s == "" || strings.EqualFold(s, "null") {
		return NoKey
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 9 {
		return NoKey
	}
	return n
}

// ParseResult reads the call results of a webhook body (one object or a
// list of them).
func ParseResult(body []byte) ([]Result, error) {
	body = bytes.TrimSpace(body)
	var wires []wireResult
	switch {
	case len(body) > 0 && body[0] == '[':
		if err := json.Unmarshal(body, &wires); err != nil {
			return nil, ErrMalformed
		}
	default:
		var w wireResult
		if err := json.Unmarshal(body, &w); err != nil {
			return nil, ErrMalformed
		}
		wires = []wireResult{w}
	}
	out := make([]Result, 0, len(wires))
	for _, w := range wires {
		if w.BulkID == "" {
			return nil, ErrMalformed
		}
		state, err := strconv.Atoi(string(w.State))
		if err != nil {
			return nil, ErrMalformed
		}
		key := keyOf(w.PushButton)
		if key == NoKey && len(w.Detail) > 0 {
			var d wireDetail
			var list []wireDetail
			switch {
			case json.Unmarshal(w.Detail, &d) == nil:
				key = keyOf(d.PushButton)
			case json.Unmarshal(w.Detail, &list) == nil && len(list) > 0:
				key = keyOf(list[0].PushButton)
			}
		}
		out = append(out, Result{
			BulkID: string(w.BulkID), Callee: string(w.Callee), State: state, Key: key,
			AnsweredAt: parseTime(w.AnswerTime), HungUpAt: parseTime(w.HangupTime),
		})
	}
	return out, nil
}

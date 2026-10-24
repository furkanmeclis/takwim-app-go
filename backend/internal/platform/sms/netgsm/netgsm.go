// Package netgsm is the Netgsm SMS adapter (TEC-982, requirements §16 R2):
// a business connects its own Netgsm account (user code, API password and
// the message header registered on the account, sealed by the integration
// vault). Takwim sends each text through the REST API, keeps the job id
// Netgsm answers with as the attempt's provider reference and reads the
// delivery report of that job (delivered, waiting, failed) from the report
// query; Netgsm pushes no SMS delivery report of its own.
//
// The adapter is registered only when SMS_NETGSM_ENABLED is true.
package netgsm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	voicenetgsm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/voice/netgsm"
)

// ProviderCode is the provider code of Netgsm (shared with the voice
// adapter: one account, two kinds).
const ProviderCode = voicenetgsm.ProviderCode

// DefaultBaseURL is Netgsm's API origin.
const DefaultBaseURL = voicenetgsm.DefaultBaseURL

// Credential fields.
const (
	FieldUserCode = voicenetgsm.FieldUserCode
	FieldPassword = voicenetgsm.FieldPassword
	// FieldHeader is the message header (sender name) registered on the
	// account; Netgsm refuses a text under another one.
	FieldHeader = "msgheader"
)

// MaxText bounds one message (Netgsm joins longer texts into several
// parts; the notification texts are capped well below).
const MaxText = 1000

// Location is the time zone Netgsm reports dates in.
var Location = voicenetgsm.Location

// Recipient turns a Turkish number into Netgsm's form (ten digits, no
// country code or leading zero); "" when it is not one.
func Recipient(phone string) string { return voicenetgsm.Recipient(phone) }

// Provider is the Netgsm SMS client and the integrations adapter.
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
func (p *Provider) Kind() string { return integrations.KindSMS }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: FieldUserCode, LabelKey: "integrations.fields.usercode", Required: true},
		{Key: FieldPassword, LabelKey: "integrations.fields.password", Secret: true, Required: true},
		{Key: FieldHeader, LabelKey: "integrations.fields.msgheader", Required: true},
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

// codeMessages are Netgsm's SMS error codes.
var codeMessages = map[string]string{
	"20":  "message text too long or not allowed",
	"30":  "invalid credentials, no API access or IP not allowed",
	"40":  "message header not registered on the account",
	"50":  "recipient refused by the IYS (consent) filter",
	"51":  "no IYS brand on the account",
	"60":  "no records",
	"70":  "invalid or missing parameter",
	"80":  "send limit exceeded",
	"85":  "repeated send limit exceeded",
	"100": "system error",
	"101": "system error",
}

func codeError(code string) *Error {
	msg, ok := codeMessages[code]
	if !ok {
		msg = "unexpected answer"
	}
	return &Error{Code: code, Message: msg}
}

// accepted are the answer codes of a send that Netgsm took (01 and 02 mean
// the dates were corrected, the text still goes).
func accepted(code string) bool { return code == "00" || code == "01" || code == "02" }

func credentials(account integrations.Account) (string, string, error) {
	u, pw := strings.TrimSpace(account.Credentials[FieldUserCode]), account.Credentials[FieldPassword]
	if u == "" || pw == "" {
		return "", "", &Error{Message: "credentials are not set"}
	}
	return u, pw, nil
}

// Check implements integrations.Adapter: lists the message headers of the
// account with its credentials (nothing is sent) and makes sure the
// account's header is one of them.
func (p *Provider) Check(ctx context.Context, account integrations.Account) error {
	u, pw, err := credentials(account)
	if err != nil {
		return err
	}
	header := strings.TrimSpace(account.Credentials[FieldHeader])
	if header == "" {
		return &Error{Message: "msgheader is required"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/sms/rest/v2/msgheader", nil)
	if err != nil {
		return errors.New("netgsm: request could not be built")
	}
	var out struct {
		Code    flex   `json:"code"`
		Headers []flex `json:"msgheader"`
	}
	if err := p.do(req, u, pw, &out); err != nil {
		return err
	}
	if !accepted(string(out.Code)) {
		return codeError(string(out.Code))
	}
	if len(out.Headers) == 0 {
		return nil // the answer lists nothing: the send tells
	}
	for _, h := range out.Headers {
		if strings.EqualFold(strings.TrimSpace(string(h)), header) {
			return nil
		}
	}
	return codeError("40")
}

// Message is one text to one phone.
type Message struct {
	To   string
	Text string
}

// Send sends one text and answers Netgsm's job id (the reference of the
// delivery report).
func (p *Provider) Send(ctx context.Context, account integrations.Account, m Message) (string, error) {
	u, pw, err := credentials(account)
	if err != nil {
		return "", err
	}
	header := strings.TrimSpace(account.Credentials[FieldHeader])
	if header == "" {
		return "", &Error{Message: "msgheader is not set"}
	}
	to := Recipient(m.To)
	if to == "" {
		return "", &Error{Message: "not a Turkish phone number"}
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return "", &Error{Message: "no text to send"}
	}
	if len([]rune(text)) > MaxText {
		return "", codeError("20")
	}
	body, err := json.Marshal(map[string]any{
		"msgheader": header,
		"messages":  []map[string]string{{"msg": text, "no": to}},
		"encoding":  "TR",
		"iysfilter": "",
	})
	if err != nil {
		return "", errors.New("netgsm: request could not be built")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/sms/rest/v2/send", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("netgsm: request could not be built")
	}
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Code        flex `json:"code"`
		JobID       flex `json:"jobid"`
		Description flex `json:"description"`
	}
	if err := p.do(req, u, pw, &out); err != nil {
		return "", err
	}
	if !accepted(string(out.Code)) {
		return "", codeError(string(out.Code))
	}
	if out.JobID == "" {
		return "", &Error{Code: string(out.Code), Message: "answer without a job id"}
	}
	return string(out.JobID), nil
}

// Delivery states of a report (Netgsm's status codes, read into three
// outcomes).
const (
	// StatusWaiting: handed to the operator, no result yet (0).
	StatusWaiting = "waiting"
	// StatusDelivered: the phone took the message (1).
	StatusDelivered = "delivered"
	// StatusFailed: the message will not arrive (2 timed out, 3 invalid
	// number, 4 operator not served, 11 refused by the operator, 12 send
	// error, 13 blocked, and any other code).
	StatusFailed = "failed"
)

// Report is the delivery report of one message of a job.
type Report struct {
	JobID string
	// Recipient is the number as Netgsm reports it.
	Recipient string
	Status    string
	// Code is Netgsm's raw status code.
	Code string
	// DeliveredAt is when the phone took the message (zero when unknown).
	DeliveredAt time.Time
}

// Failed reports whether the message will not arrive.
func (r Report) Failed() bool { return r.Status == StatusFailed }

// Error is the attempt's error text of a failed report.
func (r Report) Error() string {
	switch r.Code {
	case "2":
		return "not delivered (timed out)"
	case "3":
		return "invalid number"
	case "4":
		return "operator not served"
	case "11":
		return "refused by the operator"
	case "12":
		return "send error"
	case "13":
		return "blocked by the operator"
	}
	return "not delivered (status " + r.Code + ")"
}

// statusOf reads Netgsm's status code.
func statusOf(code string) string {
	switch strings.TrimSpace(code) {
	case "0":
		return StatusWaiting
	case "1":
		return StatusDelivered
	}
	return StatusFailed
}

type wireJob struct {
	JobID         flex `json:"jobid"`
	Number        flex `json:"number"`
	Status        flex `json:"status"`
	DeliveredDate flex `json:"deliveredDate"`
}

// Reports asks the delivery reports of the given job ids (at most
// MaxReportJobs per call). A job Netgsm does not list yet is left out of
// the answer (it is still waiting); "no records" is no error.
func (p *Provider) Reports(ctx context.Context, account integrations.Account, jobIDs []string) ([]Report, error) {
	u, pw, err := credentials(account)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(jobIDs))
	for _, id := range jobIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > MaxReportJobs {
		ids = ids[:MaxReportJobs]
	}
	body, err := json.Marshal(map[string]any{"jobids": ids, "pagesize": len(ids) * 4, "pagenumber": 1})
	if err != nil {
		return nil, errors.New("netgsm: request could not be built")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/sms/rest/v2/report", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("netgsm: request could not be built")
	}
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Code     flex `json:"code"`
		Response struct {
			Jobs []wireJob `json:"jobs"`
		} `json:"response"`
	}
	if err := p.do(req, u, pw, &out); err != nil {
		return nil, err
	}
	switch code := string(out.Code); {
	case accepted(code):
	case code == "60":
		return nil, nil
	default:
		return nil, codeError(code)
	}
	reports := make([]Report, 0, len(out.Response.Jobs))
	for _, j := range out.Response.Jobs {
		if j.JobID == "" {
			continue
		}
		r := Report{JobID: string(j.JobID), Recipient: string(j.Number), Code: strings.TrimSpace(string(j.Status))}
		r.Status = statusOf(r.Code)
		if r.Status == StatusDelivered {
			r.DeliveredAt = parseTime(string(j.DeliveredDate))
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// MaxReportJobs is how many job ids one report query asks about.
const MaxReportJobs = 100

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339, "02.01.2006 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, Location); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
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

// do runs a request with the account's credentials (HTTP basic auth) and
// decodes the JSON answer into out. The error never carries the request.
func (p *Provider) do(req *http.Request, usercode, password string, out any) error {
	req.SetBasicAuth(usercode, password)
	req.Header.Set("Accept", "application/json")
	res, err := p.Client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("netgsm: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return errors.New("netgsm: answer could not be read")
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return codeError("30")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Netgsm answers some refusals with 4xx and the code in the body.
		var e struct {
			Code flex `json:"code"`
		}
		if json.Unmarshal(body, &e) == nil && e.Code != "" {
			return codeError(string(e.Code))
		}
		return &Error{Message: "http " + strconv.Itoa(res.StatusCode)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		// The legacy endpoints answer plain codes ("30").
		if code := strings.TrimSpace(string(body)); code != "" && len(code) <= 3 {
			return codeError(code)
		}
		return &Error{Message: "answer is not JSON"}
	}
	return nil
}

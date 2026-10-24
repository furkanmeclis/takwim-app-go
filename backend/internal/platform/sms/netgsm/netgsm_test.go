package netgsm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

func account() integrations.Account {
	return integrations.Account{Kind: integrations.KindSMS, Provider: ProviderCode, Credentials: integrations.Credentials{
		FieldUserCode: "8508500000", FieldPassword: "p&ss<word>", FieldHeader: "TAKWIM",
	}}
}

func TestSendBuildsTheRequest(t *testing.T) {
	var (
		path, auth, ctype string
		body              map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ctype = r.Method+" "+r.URL.Path, r.Header.Get("Content-Type")
		u, pw, _ := r.BasicAuth()
		auth = u + ":" + pw
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, `{"code":"00","jobid":"175343083","description":"Success"}`)
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	id, err := p.Send(context.Background(), account(), Message{To: "+90 532 123 45 67", Text: " Merhaba Ayşe, randevunuz 14:30. "})
	if err != nil || id != "175343083" {
		t.Fatalf("send = %q %v", id, err)
	}
	if path != "POST /sms/rest/v2/send" || auth != "8508500000:p&ss<word>" || !strings.HasPrefix(ctype, "application/json") {
		t.Fatalf("request = %s %s %s", path, auth, ctype)
	}
	if body["msgheader"] != "TAKWIM" || body["encoding"] != "TR" {
		t.Fatalf("body = %v", body)
	}
	msgs := body["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["no"] != "5321234567" || msgs[0].(map[string]any)["msg"] != "Merhaba Ayşe, randevunuz 14:30." {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestSendRefusals(t *testing.T) {
	answer := `{"code":"40","description":"header"}`
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	check := func(want string) {
		t.Helper()
		_, err := p.Send(context.Background(), account(), Message{To: "+905321234567", Text: "x"})
		var e *Error
		if !errors.As(err, &e) || e.Code != want {
			t.Fatalf("err = %v, want code %s", err, want)
		}
	}
	check("40")
	answer, status = `{"code":"30"}`, http.StatusUnauthorized
	check("30")
	answer, status = `{"code":"00","description":"no id"}`, http.StatusOK
	check("00")
	answer = `30`
	check("30")

	// Local refusals never reach the provider.
	if _, err := p.Send(context.Background(), account(), Message{To: "+441234567890", Text: "x"}); err == nil || !strings.Contains(err.Error(), "Turkish") {
		t.Fatalf("foreign number = %v", err)
	}
	acc := account()
	delete(acc.Credentials, FieldHeader)
	if _, err := p.Send(context.Background(), acc, Message{To: "+905321234567", Text: "x"}); err == nil || !strings.Contains(err.Error(), "msgheader") {
		t.Fatalf("no header = %v", err)
	}
	if _, err := p.Send(context.Background(), account(), Message{To: "+905321234567", Text: strings.Repeat("a", MaxText+1)}); err == nil || !strings.Contains(err.Error(), "20") {
		t.Fatalf("long text = %v", err)
	}
}

func TestCheck(t *testing.T) {
	answer := `{"code":"00","msgheader":["TAKWIM","8508500000"]}`
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.Path
		_, _ = io.WriteString(w, answer)
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	if err := p.Check(context.Background(), account()); err != nil {
		t.Fatalf("check = %v", err)
	}
	if got != "GET /sms/rest/v2/msgheader" {
		t.Fatalf("request = %s", got)
	}
	answer = `{"code":"00","msgheader":["OTHER"]}`
	if err := p.Check(context.Background(), account()); err == nil || !strings.Contains(err.Error(), "40") {
		t.Fatalf("unknown header = %v", err)
	}
	answer = `{"code":"30"}`
	if err := p.Check(context.Background(), account()); err == nil || !strings.Contains(err.Error(), "30") {
		t.Fatalf("bad credentials = %v", err)
	}
	acc := account()
	acc.Credentials[FieldPassword] = ""
	if err := p.Check(context.Background(), acc); err == nil {
		t.Fatal("missing password accepted")
	}
}

func TestReports(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sms/rest/v2/report" {
			t.Errorf("path = %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, `{"code":"00","description":"ok","response":{"jobs":[
			{"jobid":"1","number":"5321234567","status":1,"operator":"TURKCELL","deliveredDate":"2030-01-13 11:05:30"},
			{"jobid":"2","number":"5321234568","status":"0"},
			{"jobid":"3","number":"5321234569","status":"3"},
			{"jobid":"4","number":"5321234560","status":"42"}
		]}}`)
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	reports, err := p.Reports(context.Background(), account(), []string{"1", "", "2", "3", "4"})
	if err != nil || len(reports) != 4 {
		t.Fatalf("reports = %+v %v", reports, err)
	}
	if ids := body["jobids"].([]any); len(ids) != 4 || ids[0] != "1" {
		t.Fatalf("jobids = %v", ids)
	}
	want := time.Date(2030, 1, 13, 8, 5, 30, 0, time.UTC)
	if reports[0].Status != StatusDelivered || !reports[0].DeliveredAt.Equal(want) || reports[0].Recipient != "5321234567" {
		t.Fatalf("delivered = %+v", reports[0])
	}
	if reports[1].Status != StatusWaiting || reports[1].Failed() {
		t.Fatalf("waiting = %+v", reports[1])
	}
	if !reports[2].Failed() || reports[2].Error() != "invalid number" {
		t.Fatalf("invalid = %+v", reports[2])
	}
	if !reports[3].Failed() || !strings.Contains(reports[3].Error(), "42") {
		t.Fatalf("unknown = %+v", reports[3])
	}
	if out, err := p.Reports(context.Background(), account(), nil); err != nil || out != nil {
		t.Fatalf("no ids = %v %v", out, err)
	}
}

func TestReportsNoRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":"60","description":"no records"}`)
	}))
	defer srv.Close()
	reports, err := New(srv.URL, nil).Reports(context.Background(), account(), []string{"1"})
	if err != nil || len(reports) != 0 {
		t.Fatalf("reports = %v %v", reports, err)
	}
}

func TestSender(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":"00","jobid":"9"}`)
	}))
	defer srv.Close()
	s := &Sender{Client: New(srv.URL, nil), Account: account()}
	if name, ready := sms.Describe(s); name != ProviderCode || !ready {
		t.Fatalf("describe = %s %v", name, ready)
	}
	if err := s.Send(context.Background(), sms.Message{To: "+905321234567", Text: "x", Purpose: sms.PurposeCustomerOTP}); err != nil {
		t.Fatal(err)
	}
	var empty *Sender
	if _, ready := sms.Describe(empty); ready {
		t.Fatal("nil sender ready")
	}
	if err := empty.Send(context.Background(), sms.Message{}); !errors.Is(err, sms.ErrUnavailable) {
		t.Fatalf("nil send = %v", err)
	}
}

// TestLiveSend is the real integration test of A09 for SMS: it sends one
// text from a real Netgsm account and reads its delivery report. It runs
// only when the account is given:
//
//	SMS_IT_USERCODE, SMS_IT_PASSWORD, SMS_IT_HEADER, SMS_IT_TO
//	(+ SMS_IT_BASE_URL when not the default)
//
// The end-to-end check (a booking of a test business with the account
// connected) is in docs/design/confirmations.md, "Real integration test".
func TestLiveSend(t *testing.T) {
	u, pw, header, to := os.Getenv("SMS_IT_USERCODE"), os.Getenv("SMS_IT_PASSWORD"), os.Getenv("SMS_IT_HEADER"), os.Getenv("SMS_IT_TO")
	if u == "" || pw == "" || header == "" || to == "" {
		t.Skip("SMS_IT_* not set")
	}
	acc := integrations.Account{Credentials: integrations.Credentials{FieldUserCode: u, FieldPassword: pw, FieldHeader: header}}
	p := New(os.Getenv("SMS_IT_BASE_URL"), nil)
	ctx := t.Context()
	if err := p.Check(ctx, acc); err != nil {
		t.Fatalf("check: %v", err)
	}
	id, err := p.Send(ctx, acc, Message{To: to, Text: "Takwim test: randevunuzu teyit etmek icin https://takwim.link/panel/v#test"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent job %s", id)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		reports, err := p.Reports(ctx, acc, []string{id})
		if err != nil {
			t.Fatalf("report: %v", err)
		}
		for _, r := range reports {
			t.Logf("report %+v", r)
			if r.Status != StatusWaiting {
				return
			}
		}
		time.Sleep(10 * time.Second)
	}
	t.Log("no final report within 90 seconds (the receipt timeout covers it)")
}

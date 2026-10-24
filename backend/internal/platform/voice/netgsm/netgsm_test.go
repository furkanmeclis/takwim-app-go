package netgsm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

func account() integrations.Account {
	return integrations.Account{Credentials: integrations.Credentials{FieldUserCode: "8508500000", FieldPassword: "p&ss<word>"}}
}

func TestRecipient(t *testing.T) {
	for in, want := range map[string]string{
		"+905321234567": "5321234567", "05321234567": "5321234567", "5321234567": "5321234567",
		"+90 (212) 555 00 00": "2125550000", "+441234567890": "", "123": "", "0000000000": "",
	} {
		if got := Recipient(in); got != want {
			t.Errorf("Recipient(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSendBuildsTheOrder(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/voicesms/send" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = io.WriteString(w, "00 175343083")
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	start := time.Date(2030, 1, 13, 8, 5, 0, 0, time.UTC) // 11:05 in Istanbul
	id, err := p.Send(context.Background(), account(), Call{
		To: "+905321234567", Text: "Merhaba Ayşe & Ali. 1'e basın.",
		Keys:  []Key{{Digit: 1, Text: "Teşekkürler"}, {Digit: 2, Text: "İptal <alındı>"}, {Digit: 12, Text: "x"}},
		Start: start, Stop: start.Add(10 * time.Minute), URL: "https://app.example/api/v1/public/voice/netgsm/a/webhook?d=1&s=2",
	})
	if err != nil || id != "175343083" {
		t.Fatalf("send = %q %v", id, err)
	}
	for _, want := range []string{
		"<usercode>8508500000</usercode>", "<password>p&amp;ss&lt;word&gt;</password>",
		"<startdate>13012030</startdate>", "<starttime>1105</starttime>",
		// Netgsm takes at least an hour.
		"<stopdate>13012030</stopdate>", "<stoptime>1205</stoptime>",
		"<key>1</key>", "<ringtime>30</ringtime>",
		"<url>https://app.example/api/v1/public/voice/netgsm/a/webhook?d=1&amp;s=2</url>",
		"<text>Merhaba Ayşe &amp; Ali. 1&#39;e basın.</text>", "<no>5321234567</no>",
		"<keys><keydetail><keyinfo>1</keyinfo><text>Teşekkürler</text></keydetail></keys>",
		"<keyinfo>2</keyinfo><text>İptal &lt;alındı&gt;</text>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("order lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<keyinfo>12</keyinfo>") {
		t.Error("a key that is not a digit was sent")
	}
}

func TestSendErrors(t *testing.T) {
	answer := "30"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, answer)
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	call := Call{To: "05321234567", Text: "x", Start: time.Now(), Stop: time.Now().Add(time.Hour)}
	_, err := p.Send(context.Background(), account(), call)
	if err == nil || !strings.Contains(err.Error(), "30") {
		t.Fatalf("code 30 = %v", err)
	}
	if strings.Contains(err.Error(), "p&ss") {
		t.Fatal("the error carries the password")
	}
	if _, err := p.Send(context.Background(), account(), Call{To: "+441234567890", Text: "x"}); err == nil {
		t.Fatal("a foreign number was ordered")
	}
	// A transport error never shows the URL.
	down := New("http://127.0.0.1:1", nil)
	if err := down.Check(context.Background(), account()); err == nil || strings.Contains(err.Error(), "password") {
		t.Fatalf("check error = %v", err)
	}
}

func TestCheck(t *testing.T) {
	answer := "60"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/voicesms/report/" || r.URL.Query().Get("usercode") != "8508500000" || r.URL.Query().Get("type") != "2" {
			t.Errorf("check request = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, answer)
	}))
	defer srv.Close()
	p := New(srv.URL, nil)
	if err := p.Check(context.Background(), account()); err != nil {
		t.Fatalf("no records = %v", err)
	}
	answer = "30"
	if err := p.Check(context.Background(), account()); err == nil {
		t.Fatal("wrong credentials passed the check")
	}
	answer = "175343083 5321234567 1 1<br>"
	if err := p.Check(context.Background(), account()); err != nil {
		t.Fatalf("records = %v", err)
	}
}

func TestParseResult(t *testing.T) {
	rs, err := ParseResult([]byte(`{"bulkid":"175343083","caller":"8508500000","callee":"5321234567","state":1,"type":3,
		"answer_time":"2030-01-13 11:06:01.120","hangup_time":"2030-01-13 11:06:40.000","bilsec":39,
		"detail":{"push_button":"2","survey_taskid":0}}`))
	if err != nil || len(rs) != 1 {
		t.Fatalf("parse = %v %v", rs, err)
	}
	r := rs[0]
	if r.BulkID != "175343083" || r.State != StateAnswered || r.Key != 2 || r.Callee != "5321234567" ||
		!r.AnsweredAt.Equal(time.Date(2030, 1, 13, 8, 6, 1, 120e6, time.UTC)) {
		t.Fatalf("result = %+v", r)
	}
	// A list, numbers as numbers, no key (null and -1), a detail list.
	rs, err = ParseResult([]byte(`[{"bulkid":1,"state":"2"},{"bulkid":"2","state":1,"detail":{"push_button":null}},
		{"bulkid":"3","state":1,"detail":[{"push_button":-1}]},{"bulkid":"4","state":1,"detail":[{"push_button":3}]}]`))
	if err != nil || len(rs) != 4 {
		t.Fatalf("list = %v %v", rs, err)
	}
	if rs[0].BulkID != "1" || rs[0].State != StateNotAnswered || rs[1].Key != NoKey || rs[2].Key != NoKey || rs[3].Key != 3 {
		t.Fatalf("list = %+v", rs)
	}
	for _, bad := range []string{`nope`, `{"state":1}`, `{"bulkid":"1","state":"x"}`} {
		if _, err := ParseResult([]byte(bad)); err == nil {
			t.Errorf("%s parsed", bad)
		}
	}
}

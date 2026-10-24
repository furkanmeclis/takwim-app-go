package meta_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/meta"
)

func account() integrations.Account {
	return integrations.Account{Kind: integrations.KindWhatsApp, Provider: meta.ProviderCode, Credentials: integrations.Credentials{
		meta.FieldPhoneNumberID: "1234567890", meta.FieldAccessToken: "token-1", meta.FieldAppSecret: "secret-1",
		meta.FieldVerifyToken: "verify-1",
	}}
}

func TestSendTemplate(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v21.0/1234567890/messages" || r.Header.Get("Authorization") != "Bearer token-1" {
			t.Errorf("request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.OUT1"}]}`))
	}))
	defer srv.Close()
	p := meta.New(srv.URL, "", nil)
	name, lang := meta.Template(account())
	id, err := p.SendTemplate(t.Context(), account(), meta.TemplateMessage{
		To: "+90 532 111 22 33", Name: name, Language: lang, BodyParams: []string{"Ayşe", ""},
		QuickReplies: []string{"a", "b", "c"},
	})
	if err != nil || id != "wamid.OUT1" {
		t.Fatalf("send = %q %v", id, err)
	}
	if got["to"] != "905321112233" || got["type"] != "template" {
		t.Fatalf("body = %v", got)
	}
	tpl := got["template"].(map[string]any)
	if tpl["name"] != meta.DefaultTemplate || tpl["language"].(map[string]any)["code"] != "tr" {
		t.Fatalf("template = %v", tpl)
	}
	comps := tpl["components"].([]any)
	if len(comps) != 4 {
		t.Fatalf("components = %v", comps)
	}
	params := comps[0].(map[string]any)["parameters"].([]any)
	if params[1].(map[string]any)["text"] != "-" {
		t.Fatalf("empty parameter = %v", params)
	}
	btn := comps[3].(map[string]any)
	if btn["sub_type"] != "quick_reply" || btn["index"] != "2" || btn["parameters"].([]any)[0].(map[string]any)["payload"] != "c" {
		t.Fatalf("button = %v", btn)
	}
}

func TestSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Template name does not exist","code":132001}}`))
	}))
	defer srv.Close()
	_, err := meta.New(srv.URL, "", nil).SendText(t.Context(), account(), "+905321112233", "x")
	var e *meta.Error
	if err == nil || !strings.Contains(err.Error(), "132001") {
		t.Fatalf("err = %v", err)
	}
	if !errors.As(err, &e) || e.Status != http.StatusBadRequest {
		t.Fatalf("error = %#v", err)
	}
}

func TestSignatureAndChallenge(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account"}`)
	sig := meta.Sign("secret-1", body)
	if !meta.VerifySignature(account(), sig, body) {
		t.Fatal("valid signature refused")
	}
	if meta.VerifySignature(account(), meta.Sign("other", body), body) || meta.VerifySignature(account(), "", body) ||
		meta.VerifySignature(account(), sig, append(body, ' ')) {
		t.Fatal("invalid signature accepted")
	}
	if !meta.VerifyChallenge(account(), "subscribe", "verify-1") || meta.VerifyChallenge(account(), "subscribe", "x") ||
		meta.VerifyChallenge(account(), "unsubscribe", "verify-1") {
		t.Fatal("challenge")
	}
}

func TestParseWebhook(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"1","changes":[{"field":"messages","value":{
		"messaging_product":"whatsapp","metadata":{"phone_number_id":"1234567890"},
		"statuses":[{"id":"wamid.OUT1","status":"delivered","timestamp":"1761300000","recipient_id":"905321112233"},
			{"id":"wamid.OUT2","status":"failed","timestamp":"1761300001","errors":[{"code":131026,"title":"Message undeliverable"}]}],
		"messages":[{"from":"905321112233","id":"wamid.IN1","timestamp":"1761300100","type":"button",
			"context":{"from":"905000000000","id":"wamid.OUT1"},"button":{"payload":"tkw1:confirm","text":"Geleceğim"}},
			{"from":"905321112233","id":"wamid.IN2","timestamp":"1761300200","type":"text","text":{"body":"evet"}}]}}]}]}`)
	w, err := meta.ParseWebhook(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Statuses) != 2 || w.Statuses[0].Status != meta.StatusDelivered || w.Statuses[0].At.Unix() != 1761300000 ||
		w.Statuses[1].Error != "131026 Message undeliverable" || w.Statuses[0].PhoneNumberID != "1234567890" {
		t.Fatalf("statuses = %+v", w.Statuses)
	}
	if len(w.Replies) != 2 || w.Replies[0].Payload != "tkw1:confirm" || w.Replies[0].ContextID != "wamid.OUT1" ||
		w.Replies[1].Payload != "" || w.Replies[1].Text != "evet" {
		t.Fatalf("replies = %+v", w.Replies)
	}
	if _, err := meta.ParseWebhook([]byte(`{"object":"page"}`)); err == nil {
		t.Fatal("other object accepted")
	}
}

// TestLiveTemplate is the real integration test of A09: it sends the
// confirmation template from a real WhatsApp Business number. It runs only
// when the account is given:
//
//	WHATSAPP_IT_PHONE_NUMBER_ID, WHATSAPP_IT_ACCESS_TOKEN, WHATSAPP_IT_TO
//	(+ WHATSAPP_IT_TEMPLATE, WHATSAPP_IT_LANGUAGE when not the defaults)
//
// The buttons' answers are then checked on the webhook (see
// docs/design/confirmations.md, "Channels").
func TestLiveTemplate(t *testing.T) {
	id, token, to := os.Getenv("WHATSAPP_IT_PHONE_NUMBER_ID"), os.Getenv("WHATSAPP_IT_ACCESS_TOKEN"), os.Getenv("WHATSAPP_IT_TO")
	if id == "" || token == "" || to == "" {
		t.Skip("WHATSAPP_IT_* not set")
	}
	acc := integrations.Account{Credentials: integrations.Credentials{
		meta.FieldPhoneNumberID: id, meta.FieldAccessToken: token, meta.FieldAppSecret: "-", meta.FieldVerifyToken: "-",
		meta.FieldTemplate: os.Getenv("WHATSAPP_IT_TEMPLATE"), meta.FieldLanguage: os.Getenv("WHATSAPP_IT_LANGUAGE"),
	}}
	p := meta.New(os.Getenv("WHATSAPP_IT_GRAPH_URL"), "", nil)
	if err := p.Check(t.Context(), acc); err != nil {
		t.Fatalf("check: %v", err)
	}
	name, lang := meta.Template(acc)
	msg, err := p.SendTemplate(t.Context(), acc, meta.TemplateMessage{
		To: to, Name: name, Language: lang,
		BodyParams:   []string{"Test Müşteri", "Takwim Test", "24.10.2026 14:30", "24.10.2026 12:30", "1042"},
		QuickReplies: []string{"tkw1:confirm", "tkw1:cancel", "tkw1:change"},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent %s", msg)
}

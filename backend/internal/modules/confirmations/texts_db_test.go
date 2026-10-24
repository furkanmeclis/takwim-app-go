package confirmations_test

import (
	"context"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	notifymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/model"
	notifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
)

func detailCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	ve, ok := notifyusecase.IsValidation(err)
	if !ok {
		t.Fatalf("want a validation error, got %v", err)
	}
	out := map[string]string{}
	for _, d := range ve.Details {
		out[d.Field] = d.Code
	}
	return out
}

// TestRequestTexts (TEC-979): the business's SMS and e-mail texts of the
// confirmation request are saved apart from the rule, keep the visit link,
// go out with the request and go back to the default on reset. WhatsApp
// (the approved template) and the voice call (the catalog text) are not
// editable.
func TestRequestTexts(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	code := catalog.ConfirmationRequest

	texts, err := f.notify.ManagedTexts(ctx, f.org.ID, code)
	if err != nil {
		t.Fatal(err)
	}
	if texts.Custom || len(texts.Templates) != 2 || texts.Templates[0].Channel != "sms" || texts.Templates[1].Channel != "email" ||
		texts.Templates[0].Body != texts.Templates[0].DefaultBody || texts.Voice == "" {
		t.Fatalf("defaults %+v", texts)
	}
	hasDeadline := false
	for _, v := range texts.Variables {
		hasDeadline = hasDeadline || v == "deadline"
	}
	if !hasDeadline || len(texts.Required) != 1 || texts.Required[0] != "link" {
		t.Fatalf("variables %v required %v", texts.Variables, texts.Required)
	}

	// Refused: no link, an unknown variable, WhatsApp and the call, nothing.
	_, err = f.notify.SaveManagedTexts(ctx, f.org.ID, f.owner.ID, code, notifymodel.ManagedTextsInput{Templates: []notifymodel.TemplateInput{
		{Channel: "sms", Body: "Lütfen {{deadline}} saatine kadar teyit edin."},
		{Channel: "email", Subject: "Teyit", Body: "{{link}} {{price}}"},
		{Channel: "whatsapp", Body: "x {{link}}"},
		{Channel: "voice_call", Body: "x {{link}}"},
	}})
	got := detailCodes(t, err)
	if got["templates.0.body"] != "missing_variable" || got["templates.1.body"] != "unknown_variable" ||
		got["templates.2.channel"] != "invalid" || got["templates.3.channel"] != "invalid" {
		t.Fatalf("details %v", got)
	}
	if _, err = f.notify.SaveManagedTexts(ctx, f.org.ID, f.owner.ID, code, notifymodel.ManagedTextsInput{}); err == nil {
		t.Fatal("empty save accepted")
	}
	// The notification settings still refuse the managed event.
	if _, err := f.notify.Save(ctx, f.org.ID, f.owner.ID, notifymodel.SaveInput{Event: code, Channels: []string{"sms"}}); err == nil {
		t.Fatal("notification settings saved the confirmation rule")
	}

	custom := "Merhaba {{customer_name}}, {{deadline}} saatine kadar yanıtlayın: {{link}} TKW-OZEL"
	texts, err = f.notify.SaveManagedTexts(ctx, f.org.ID, f.owner.ID, code, notifymodel.ManagedTextsInput{Templates: []notifymodel.TemplateInput{
		{Channel: "sms", Body: custom},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !texts.Custom || !texts.Templates[0].Custom || texts.Templates[0].Body != custom || texts.Templates[1].Custom || texts.UpdatedAt == nil {
		t.Fatalf("saved %+v", texts)
	}

	// The preview knows the deadline.
	p, err := f.notify.Preview(ctx, f.org.ID, notifymodel.PreviewInput{Event: code, Channel: "sms", Body: custom})
	if err != nil || len(p.Unknown) != 0 {
		t.Fatalf("preview %+v %v", p, err)
	}

	// The request goes out with the business's text, the visit link and
	// the deadline.
	r := f.book(t, at(7, 14, 0))
	f.tick(t, at(6, 14, 0))
	if f.requests(r) != 1 || f.sms.texts("TKW-OZEL") != 1 || f.sms.texts("Merhaba Zeynep") != 1 || f.sms.texts("07.01.2030 12:00 saatine") != 1 ||
		f.sms.texts("panel/v#visit-token TKW-OZEL") != 1 {
		t.Fatalf("sms %+v", f.sms.msgs)
	}

	// The default text saved again, or a reset, goes back to the default.
	def := texts.Templates[0].DefaultBody
	if texts, err = f.notify.SaveManagedTexts(ctx, f.org.ID, f.owner.ID, code, notifymodel.ManagedTextsInput{Templates: []notifymodel.TemplateInput{
		{Channel: "sms", Body: def},
	}}); err != nil || texts.Custom {
		t.Fatalf("default text kept as custom: %+v %v", texts, err)
	}
	if _, err = f.notify.SaveManagedTexts(ctx, f.org.ID, f.owner.ID, code, notifymodel.ManagedTextsInput{Templates: []notifymodel.TemplateInput{
		{Channel: "email", Subject: "Teyit ({{business_name}})", Body: "Teyit için: {{link}}"},
	}}); err != nil {
		t.Fatal(err)
	}
	if texts, err = f.notify.ResetManagedTexts(ctx, f.org.ID, code); err != nil || texts.Custom || texts.Templates[1].Body != texts.Templates[1].DefaultBody {
		t.Fatalf("reset %+v %v", texts, err)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_templates WHERE organization_id = $1`, f.org.ID); n != 0 {
		t.Fatalf("%d texts left after reset", n)
	}
}

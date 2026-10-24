// Package catalog lists the customer notification events (TEC-911): the
// default rule (ordered channels, fallback, reminder offsets), the
// variables a text may use and the default Turkish texts.
//
// Messages carry names, the booking number, times, the branch, the
// services and the public panel address (which asks for a sign-in code):
// never prices, notes or another person's data.
package catalog

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
)

// Event codes.
const (
	ReservationRequested   = "reservation.requested"
	ReservationAccepted    = "reservation.accepted"
	ReservationRejected    = "reservation.rejected"
	ReservationRescheduled = "reservation.rescheduled"
	ReservationCancelled   = "reservation.cancelled"
	ReservationReminder    = "reservation.reminder"
	// ConfirmationRequest asks the customer to confirm a booking (the
	// Konfirmasyon add-on, TEC-921). Its channels and timing come from the
	// confirmation settings, not from a notification rule, so it is not in
	// All() (the notification settings page).
	ConfirmationRequest = "confirmation.request"
)

// Limits of a rule.
const (
	// MaxOffsets is the number of reminders a rule may plan.
	MaxOffsets = 3
	// MinOffsetMinutes / MaxOffsetMinutes bound a reminder offset (15
	// minutes to 7 days before the visit).
	MinOffsetMinutes = 15
	MaxOffsetMinutes = 7 * 24 * 60
	// MaxSubject / MaxBody bound a business's text.
	MaxSubject = 200
	MaxBody    = 1000
)

// Template is a default text. Subject is used by e-mail only.
type Template struct {
	Subject string
	Body    string
}

// Event is one catalog entry.
type Event struct {
	Code string
	// Reminder events fire before the visit at the rule's offsets; the
	// others when the reservation changes.
	Reminder bool
	Channels []string
	Offsets  []int32
	// Text is the SMS / WhatsApp default; Email the e-mail default.
	Text  string
	Email Template
	// Voice is what an automatic call reads out (TEC-923); empty = the
	// event is never called.
	Voice string
}

// Vars every reservation text may use ({{name}}).
var Vars = []string{
	"customer_name", "business_name", "branch_name", "reference", "starts_at", "date", "time", "services", "link",
}

var defaultChannels = []string{channel.SMS, channel.Email}

const signature = "\n\n{{business_name}}"

var events = []Event{
	{
		Code: ReservationRequested, Channels: defaultChannels,
		Text: "{{business_name}}: {{starts_at}} için randevu talebiniz alındı (no {{reference}}). Onaylanınca bilgi vereceğiz. Randevularınız: {{link}}",
		Email: Template{
			Subject: "Randevu talebiniz alındı ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\n{{starts_at}} için randevu talebiniz alındı.\nNo: {{reference}}\nHizmet: {{services}}\nŞube: {{branch_name}}\n\n" +
				"Talebiniz onaylandığında size haber vereceğiz. Bütün randevularınızı görmek ve yönetmek için: {{link}}" + signature,
		},
	},
	{
		Code: ReservationAccepted, Channels: defaultChannels,
		Text: "{{business_name}}: {{starts_at}} randevunuz onaylandı (no {{reference}}). Görmek ve yönetmek için: {{link}}",
		Email: Template{
			Subject: "Randevunuz onaylandı ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\n{{starts_at}} randevunuz onaylandı.\nNo: {{reference}}\nHizmet: {{services}}\nŞube: {{branch_name}}\n\n" +
				"Bütün randevularınızı görmek ve yönetmek için: {{link}}" + signature,
		},
	},
	{
		Code: ReservationRejected, Channels: defaultChannels,
		Text: "{{business_name}}: {{starts_at}} için randevu talebiniz maalesef onaylanamadı (no {{reference}}). Randevularınız: {{link}}",
		Email: Template{
			Subject: "Randevu talebiniz onaylanamadı ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\n{{starts_at}} için randevu talebiniz maalesef onaylanamadı.\nNo: {{reference}}\nHizmet: {{services}}\n\n" +
				"Başka bir zaman için yeniden randevu alabilirsiniz. Randevularınız: {{link}}" + signature,
		},
	},
	{
		Code: ReservationRescheduled, Channels: defaultChannels,
		Text: "{{business_name}}: randevunuz {{starts_at}} olarak güncellendi (no {{reference}}). Ayrıntılar: {{link}}",
		Email: Template{
			Subject: "Randevunuz güncellendi ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\nrandevunuzun yeni zamanı: {{starts_at}}.\nNo: {{reference}}\nHizmet: {{services}}\nŞube: {{branch_name}}\n\n" +
				"Ayrıntılar ve değişiklik için: {{link}}" + signature,
		},
	},
	{
		Code: ReservationCancelled, Channels: defaultChannels,
		Text: "{{business_name}}: {{starts_at}} randevunuz iptal edildi (no {{reference}}). Randevularınız: {{link}}",
		Email: Template{
			Subject: "Randevunuz iptal edildi ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\n{{starts_at}} randevunuz iptal edildi.\nNo: {{reference}}\nHizmet: {{services}}\n\n" +
				"Yeni bir randevu alabilir, bütün randevularınızı burada görebilirsiniz: {{link}}" + signature,
		},
	},
	{
		Code: ReservationReminder, Reminder: true, Channels: defaultChannels, Offsets: []int32{1440},
		Text: "Hatırlatma: {{business_name}}, {{starts_at}}, {{branch_name}} (no {{reference}}). Randevunuzu görmek veya değiştirmek için: {{link}}",
		Email: Template{
			Subject: "Randevu hatırlatması ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\nrandevunuzu hatırlatırız: {{starts_at}}.\nHizmet: {{services}}\nŞube: {{branch_name}}\nNo: {{reference}}\n\n" +
				"Görmek, iptal etmek veya değiştirmek için: {{link}}" + signature,
		},
	},
}

// ConfirmationVars are the variables of the confirmation request: the
// reservation variables, the deadline of the answer and the visit link
// ({{link}} opens the one visit with "Geleceğim" / cancel / change).
var ConfirmationVars = append(append([]string(nil), Vars...), "deadline")

// managed are events whose rule lives in another module.
var managed = []Event{
	{
		Code: ConfirmationRequest, Channels: []string{channel.SMS, channel.Email},
		Text: "{{business_name}}: {{starts_at}} randevunuza geleceğinizi lütfen {{deadline}} saatine kadar teyit edin (no {{reference}}). " +
			"Teyit, iptal veya değişiklik: {{link}}",
		Email: Template{
			Subject: "Randevunuzu teyit edin ({{business_name}})",
			Body: "Merhaba {{customer_name}},\n\n{{starts_at}} randevunuza geleceğinizi lütfen {{deadline}} saatine kadar teyit edin.\n" +
				"No: {{reference}}\nHizmet: {{services}}\nŞube: {{branch_name}}\n\n" +
				"Geleceğim, iptal veya değişiklik isteği için: {{link}}\n\n" +
				"Bu süreye kadar yanıt gelmezse randevunuz işletmenin kuralına göre iptal edilebilir." + signature,
		},
		// Read out by the call; the keys and what each one says back are
		// the voice channel's (1 Geleceğim, 2 cancel, 3 change).
		Voice: "Merhaba {{customer_name}}. {{business_name}} işletmesindeki {{date}} tarihli, saat {{time}} randevunuz için arıyoruz. " +
			"Randevunuza geleceğinizi teyit etmek için 1'e, iptal etmek için 2'ye, başka bir saat istemek için 3'e basın.",
	},
}

var byCode = func() map[string]Event {
	m := make(map[string]Event, len(events)+len(managed))
	for _, e := range events {
		m[e.Code] = e
	}
	for _, e := range managed {
		m[e.Code] = e
	}
	return m
}()

// Managed reports whether an event's rule lives in another module (it is
// sent through the engine but not edited on the notification settings).
func Managed(code string) bool {
	for _, e := range managed {
		if e.Code == code {
			return true
		}
	}
	return false
}

// All returns the catalog in display order.
func All() []Event {
	out := make([]Event, len(events))
	copy(out, events)
	return out
}

// Get returns one event.
func Get(code string) (Event, bool) {
	e, ok := byCode[code]
	return e, ok
}

// DefaultTemplate is the catalog text of an event on a channel.
func DefaultTemplate(ev Event, ch string) Template {
	switch ch {
	case channel.Email:
		return ev.Email
	case channel.VoiceCall:
		return Template{Body: ev.Voice}
	}
	return Template{Body: ev.Text}
}

// SampleVars are preview values for every variable.
func SampleVars() map[string]string {
	return map[string]string{
		"customer_name": "Ayşe Yılmaz", "business_name": "Örnek Kuaför", "branch_name": "Merkez",
		"reference": "1042", "starts_at": "24.10.2026 14:30", "date": "24.10.2026", "time": "14:30",
		"services": "Saç kesimi, Fön", "link": "https://takwim.link/panel",
	}
}

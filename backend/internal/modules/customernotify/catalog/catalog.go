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

var byCode = func() map[string]Event {
	m := make(map[string]Event, len(events))
	for _, e := range events {
		m[e.Code] = e
	}
	return m
}()

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
	if ch == channel.Email {
		return ev.Email
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

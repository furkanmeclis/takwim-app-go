package usecase

import (
	"fmt"
	"math/big"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
)

// Detail is one field-level validation problem (dot path of the form field).
type Detail struct {
	Field   string
	Code    string
	Message string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

var (
	digitsRE  = regexp.MustCompile(`^[0-9]+$`)
	phoneRE   = regexp.MustCompile(`^\+?[0-9 ()\-]{7,32}$`)
	percentRE = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,2})?$`)
)

// Limits of the minute / day settings.
const (
	maxReviewWindowMinutes = 7 * 24 * 60
	maxPaymentHoldMinutes  = 7 * 24 * 60
	maxLeadMinutes         = 30 * 24 * 60
	maxAdvanceDays         = 730
	maxCutoffMinutes       = 30 * 24 * 60
	maxPolicyText          = 5000
)

// ValidVKN reports whether v is a valid Turkish tax identification number
// (Vergi Kimlik Numarası, 10 digits, GİB checksum).
func ValidVKN(v string) bool {
	if len(v) != 10 || !digitsRE.MatchString(v) {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		d := int(v[i] - '0')
		tmp := (d + 9 - i) % 10
		if tmp == 0 {
			continue
		}
		p := (tmp * (1 << (9 - i))) % 9
		if p == 0 {
			p = 9
		}
		sum += p
	}
	check := (10 - sum%10) % 10
	return check == int(v[9]-'0')
}

// ValidTCKN reports whether v is a valid Turkish national identity number
// (T.C. Kimlik No, 11 digits, first digit not zero, two check digits).
func ValidTCKN(v string) bool {
	if len(v) != 11 || !digitsRE.MatchString(v) || v[0] == '0' {
		return false
	}
	d := make([]int, 11)
	for i := range v {
		d[i] = int(v[i] - '0')
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	d10 := ((odd*7-even)%10 + 10) % 10
	if d10 != d[9] {
		return false
	}
	total := 0
	for i := 0; i < 10; i++ {
		total += d[i]
	}
	return total%10 == d[10]
}

// Normalize trims the document, fills missing channels / mappings and
// validates it. It returns the normalized settings or a *ValidationError.
func Normalize(in model.Settings) (model.Settings, error) {
	ve := &ValidationError{}
	s := in

	// --- company
	c := &s.Company
	c.LegalName = strings.TrimSpace(c.LegalName)
	c.TaxNumber = strings.ReplaceAll(strings.TrimSpace(c.TaxNumber), " ", "")
	c.TaxOffice = strings.TrimSpace(c.TaxOffice)
	c.Address = strings.TrimSpace(c.Address)
	c.Phone = strings.TrimSpace(c.Phone)
	c.Email = strings.TrimSpace(c.Email)
	c.Website = strings.TrimSpace(c.Website)
	maxLen(ve, "company.legal_name", c.LegalName, 255)
	maxLen(ve, "company.tax_office", c.TaxOffice, 128)
	maxLen(ve, "company.address", c.Address, 1000)
	switch {
	case c.TaxNumber == "":
	case len(c.TaxNumber) == 10:
		if !ValidVKN(c.TaxNumber) {
			ve.add("company.tax_number", "invalid_checksum", "tax number (VKN) checksum is invalid")
		}
	case len(c.TaxNumber) == 11:
		if !ValidTCKN(c.TaxNumber) {
			ve.add("company.tax_number", "invalid_checksum", "identity number (TCKN) checksum is invalid")
		}
	default:
		ve.add("company.tax_number", "invalid", "tax number must be a 10-digit VKN or an 11-digit TCKN")
	}
	if c.Phone != "" && !phoneRE.MatchString(c.Phone) {
		ve.add("company.phone", "invalid", "phone number is invalid")
	}
	if c.Email != "" {
		if addr, err := mail.ParseAddress(c.Email); err != nil || addr.Address != c.Email || len(c.Email) > 255 {
			ve.add("company.email", "invalid", "email is invalid")
		}
	}
	if c.Website != "" {
		if u, err := url.Parse(c.Website); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(c.Website) > 255 {
			ve.add("company.website", "invalid", "website must be an http(s) address")
		}
	}

	// --- reservation
	r := &s.Reservation
	r.ApprovalMode = strings.TrimSpace(r.ApprovalMode)
	if r.ApprovalMode != "" && r.ApprovalMode != model.ApprovalAuto && r.ApprovalMode != model.ApprovalStaff {
		ve.add("reservation.approval_mode", "invalid", "approval mode must be auto or staff")
	}
	minutes(ve, "reservation.staff_review_window_minutes", r.StaffReviewWindowMinutes, 1, maxReviewWindowMinutes)
	minutes(ve, "reservation.payment_hold_minutes", r.PaymentHoldMinutes, 1, maxPaymentHoldMinutes)
	minutes(ve, "reservation.min_lead_minutes", r.MinLeadMinutes, 0, maxLeadMinutes)
	if r.MaxAdvanceDays != nil && (*r.MaxAdvanceDays < 1 || *r.MaxAdvanceDays > maxAdvanceDays) {
		ve.add("reservation.max_advance_days", "out_of_range", fmt.Sprintf("must be between 1 and %d days", maxAdvanceDays))
	}
	minutes(ve, "reservation.cancel_cutoff_minutes", r.CancelCutoffMinutes, 0, maxCutoffMinutes)
	minutes(ve, "reservation.change_cutoff_minutes", r.ChangeCutoffMinutes, 0, maxCutoffMinutes)
	// The earliest bookable start must lie inside the booking window.
	if r.MinLeadMinutes != nil && r.MaxAdvanceDays != nil && *r.MinLeadMinutes > *r.MaxAdvanceDays*24*60 {
		ve.add("reservation.min_lead_minutes", "after_max_advance", "earliest booking must be within the booking window")
	}

	// --- payment: at least one way to pay.
	if !s.Payment.PayAtVenue && !s.Payment.PayOnline {
		ve.add("payment.pay_at_venue", "required", "enable payment at the venue or online payment")
	}

	// --- policies
	pol := &s.Policies
	for _, f := range []struct {
		field string
		v     *string
	}{
		{"policies.cancellation", &pol.Cancellation}, {"policies.no_show", &pol.NoShow}, {"policies.change", &pol.Change},
	} {
		*f.v = strings.TrimSpace(*f.v)
		maxLen(ve, f.field, *f.v, maxPolicyText)
	}

	// --- sales channels: every known channel exactly once, in order.
	active := map[string]bool{}
	seen := map[string]bool{}
	for i, ch := range s.SalesChannels {
		code := strings.TrimSpace(ch.Code)
		switch {
		case !slices.Contains(model.SalesChannels, code):
			ve.add(fmt.Sprintf("sales_channels.%d.code", i), "invalid", "unknown sales channel "+code)
		case seen[code]:
			ve.add(fmt.Sprintf("sales_channels.%d.code", i), "duplicate", "sales channel is listed twice")
		}
		seen[code] = true
		active[code] = ch.IsActive
	}
	s.SalesChannels = make([]model.SalesChannel, 0, len(model.SalesChannels))
	for _, code := range model.SalesChannels {
		s.SalesChannels = append(s.SalesChannels, model.SalesChannel{Code: code, IsActive: active[code]})
	}

	// --- embed: allowed sites, normalized and listed once.
	origins := make([]string, 0, len(s.Embed.AllowedOrigins))
	for i, raw := range s.Embed.AllowedOrigins {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		o, ok := NormalizeOrigin(raw)
		if !ok {
			ve.add(fmt.Sprintf("embed.allowed_origins.%d", i), "invalid", "must be a site address such as https://example.com")
			continue
		}
		if !slices.Contains(origins, o) {
			origins = append(origins, o)
		}
	}
	if len(origins) > model.MaxEmbedOrigins {
		ve.add("embed.allowed_origins", "too_many", fmt.Sprintf("at most %d sites", model.MaxEmbedOrigins))
	}
	s.Embed.AllowedOrigins = origins

	// --- tax
	t := &s.Tax
	t.DefaultVATRate = strings.TrimSpace(t.DefaultVATRate)
	if t.DefaultVATRate != "" && !validPercent(t.DefaultVATRate) {
		ve.add("tax.default_vat_rate", "invalid", "VAT rate must be a percentage between 0 and 100")
	}
	byType := map[string]model.InvoiceItemMapping{}
	seenType := map[string]bool{}
	for i, m := range t.InvoiceItemMappings {
		m.ItemType = strings.TrimSpace(m.ItemType)
		m.AccountCode = strings.TrimSpace(m.AccountCode)
		m.VATRate = strings.TrimSpace(m.VATRate)
		m.Description = strings.TrimSpace(m.Description)
		prefix := fmt.Sprintf("tax.invoice_item_mappings.%d", i)
		switch {
		case !slices.Contains(model.InvoiceItemTypes, m.ItemType):
			ve.add(prefix+".item_type", "invalid", "unknown invoice item type "+m.ItemType)
		case seenType[m.ItemType]:
			ve.add(prefix+".item_type", "duplicate", "invoice item type is listed twice")
		}
		seenType[m.ItemType] = true
		maxLen(ve, prefix+".account_code", m.AccountCode, 64)
		maxLen(ve, prefix+".description", m.Description, 255)
		if m.VATRate != "" && !validPercent(m.VATRate) {
			ve.add(prefix+".vat_rate", "invalid", "VAT rate must be a percentage between 0 and 100")
		}
		byType[m.ItemType] = m
	}
	t.InvoiceItemMappings = make([]model.InvoiceItemMapping, 0, len(model.InvoiceItemTypes))
	for _, it := range model.InvoiceItemTypes {
		m := byType[it]
		m.ItemType = it
		t.InvoiceItemMappings = append(t.InvoiceItemMappings, m)
	}

	if len(ve.Details) > 0 {
		return s, ve
	}
	return s, nil
}

var hostLabelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeOrigin turns a site address into the origin a browser sends
// (`https://shop.example.com`, `https://example.com:8443`). A missing scheme
// means https; http is accepted only for localhost (development). A leading
// `*.` allows every subdomain, as in a CSP source. Paths, queries,
// fragments, credentials, IP addresses other than 127.0.0.1 and bare
// top-level names are refused.
func NormalizeOrigin(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" || len(v) > 255 || strings.ContainsAny(v, " \t'\";,") {
		return "", false
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	v = strings.TrimRight(v, "/")
	wildcard := false
	if rest, ok := strings.CutPrefix(v, "https://*."); ok {
		wildcard = true
		v = "https://" + rest
	}
	u, err := url.Parse(v)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	host, port := u.Hostname(), u.Port()
	local := host == "localhost" || host == "127.0.0.1"
	switch {
	case u.Scheme == "http" && !local:
		return "", false
	case u.Scheme != "http" && u.Scheme != "https":
		return "", false
	case wildcard && local:
		return "", false
	}
	if port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", false
		}
	}
	if !local {
		labels := strings.Split(host, ".")
		if len(labels) < 2 {
			return "", false
		}
		for _, l := range labels {
			if !hostLabelRE.MatchString(l) {
				return "", false
			}
		}
		// The last label is a name (com, istanbul, xn--...), never digits.
		if digitsRE.MatchString(labels[len(labels)-1]) {
			return "", false
		}
	}
	out := u.Scheme + "://"
	if wildcard {
		out += "*."
	}
	out += host
	if port != "" {
		out += ":" + port
	}
	return out, true
}

func maxLen(ve *ValidationError, field, v string, n int) {
	if utf8.RuneCountInString(v) > n {
		ve.add(field, "too_long", fmt.Sprintf("must be at most %d characters", n))
	}
}

func minutes(ve *ValidationError, field string, v *int, lo, hi int) {
	if v == nil {
		return
	}
	if *v < lo || *v > hi {
		ve.add(field, "out_of_range", fmt.Sprintf("must be between %d and %d minutes", lo, hi))
	}
}

func validPercent(v string) bool {
	if !percentRE.MatchString(v) {
		return false
	}
	r, ok := new(big.Rat).SetString(v)
	return ok && r.Sign() >= 0 && r.Cmp(big.NewRat(100, 1)) <= 0
}

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
	// The online slot grid: a multiple of 5 minutes up to 4 hours (the
	// availability engine's step limits).
	minSlotStep = 5
	maxSlotStep = 240
	// The online booking limits.
	maxOpenPerCustomer = 100
	maxOnlinePerDay    = 10000
)

var fieldKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

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
	if v := r.SlotStepMinutes; v != nil && (*v < minSlotStep || *v > maxSlotStep || *v%minSlotStep != 0) {
		ve.add("reservation.slot_step_minutes", "out_of_range", fmt.Sprintf("must be a multiple of %d between %d and %d minutes", minSlotStep, minSlotStep, maxSlotStep))
	}
	if v := r.MaxOpenPerCustomer; v != nil && (*v < 1 || *v > maxOpenPerCustomer) {
		ve.add("reservation.max_open_per_customer", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxOpenPerCustomer))
	}
	if v := r.MaxOnlinePerDay; v != nil && (*v < 1 || *v > maxOnlinePerDay) {
		ve.add("reservation.max_online_per_day", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxOnlinePerDay))
	}
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

	s.Booking = normalizeBooking(ve, s.Booking)

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

// normalizeBooking validates the flow options and the booking form: known
// optional steps once each (in display order), at most MaxFields questions
// with unique keys (derived from the label when empty), a known type,
// options only for a select (2 or more, unique) and at least one place to
// ask them.
func normalizeBooking(ve *ValidationError, b model.Booking) model.Booking {
	hidden := map[string]bool{}
	for i, step := range b.HiddenSteps {
		step = strings.TrimSpace(step)
		if !slices.Contains(model.HideableSteps, step) {
			ve.add(fmt.Sprintf("booking.hidden_steps.%d", i), "invalid", "this step cannot be hidden: "+step)
			continue
		}
		hidden[step] = true
	}
	out := model.Booking{HiddenSteps: []string{}, Fields: []model.CustomField{}}
	for _, step := range model.HideableSteps {
		if hidden[step] {
			out.HiddenSteps = append(out.HiddenSteps, step)
		}
	}
	if len(b.Fields) > model.MaxFields {
		ve.add("booking.fields", "too_many", fmt.Sprintf("at most %d questions", model.MaxFields))
		return out
	}
	keys := map[string]bool{}
	// Keys given by the client first, so a derived key never takes one.
	for _, f := range b.Fields {
		if k := strings.TrimSpace(f.Key); k != "" {
			keys[k] = true
		}
	}
	seen := map[string]bool{}
	for i, f := range b.Fields {
		prefix := fmt.Sprintf("booking.fields.%d", i)
		f.Key = strings.TrimSpace(f.Key)
		f.Label = strings.TrimSpace(f.Label)
		f.Type = strings.TrimSpace(f.Type)
		f.Help = strings.TrimSpace(f.Help)
		if f.Label == "" {
			ve.add(prefix+".label", "required", "label is required")
		}
		maxLen(ve, prefix+".label", f.Label, model.MaxFieldLabel)
		maxLen(ve, prefix+".help", f.Help, model.MaxFieldHelp)
		if f.Key == "" {
			f.Key = uniqueKey(fieldKey(f.Label), keys)
			keys[f.Key] = true
		}
		switch {
		case !fieldKeyRE.MatchString(f.Key):
			ve.add(prefix+".key", "invalid", "key must be lower case letters, digits and _ (starting with a letter)")
		case seen[f.Key]:
			ve.add(prefix+".key", "duplicate", "key is used twice")
		}
		seen[f.Key] = true
		if !slices.Contains(model.FieldTypes, f.Type) {
			ve.add(prefix+".type", "invalid", "type must be text, number, select or checkbox")
		}
		options := []string{}
		if f.Type == model.FieldSelect {
			uniq := map[string]bool{}
			for j, o := range f.Options {
				o = strings.TrimSpace(o)
				switch {
				case o == "":
					ve.add(fmt.Sprintf("%s.options.%d", prefix, j), "required", "option is empty")
				case uniq[o]:
					ve.add(fmt.Sprintf("%s.options.%d", prefix, j), "duplicate", "option is listed twice")
				}
				maxLen(ve, fmt.Sprintf("%s.options.%d", prefix, j), o, model.MaxOptionLength)
				uniq[o] = true
				options = append(options, o)
			}
			switch {
			case len(options) < 2:
				ve.add(prefix+".options", "too_few", "a select needs at least 2 options")
			case len(options) > model.MaxFieldOptions:
				ve.add(prefix+".options", "too_many", fmt.Sprintf("at most %d options", model.MaxFieldOptions))
			}
		}
		f.Options = options
		if !f.Online && !f.Panel {
			ve.add(prefix+".online", "required", "ask the question online, in the panel or both")
		}
		out.Fields = append(out.Fields, f)
	}
	return out
}

// fieldKey derives an answer key from a label ("Alerji var mı?" →
// "alerji_var_mi").
func fieldKey(label string) string {
	repl := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "İ", "i", "ö", "o", "ş", "s", "ü", "u",
		"Ç", "c", "Ğ", "g", "Ö", "o", "Ş", "s", "Ü", "u")
	var b strings.Builder
	under := false
	for _, r := range strings.ToLower(repl.Replace(label)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			under = false
		case !under && b.Len() > 0:
			b.WriteByte('_')
			under = true
		}
	}
	key := strings.Trim(b.String(), "_")
	if len(key) > 32 {
		key = strings.Trim(key[:32], "_")
	}
	if key == "" || key[0] < 'a' || key[0] > 'z' {
		key = "field_" + key
	}
	return strings.Trim(key, "_")
}

func uniqueKey(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		k := fmt.Sprintf("%s_%d", base, i)
		if !taken[k] {
			return k
		}
	}
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

package observability

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
)

const redacted = "[redacted]"

var (
	reEmail  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	rePush   = regexp.MustCompile(`(?i)(?:Exponent|Expo)PushToken(?:\[|%5B)[^\]%\s]+(?:\]|%5D)?`)
	reJWT    = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]*`)
	reBearer = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=\-]+`)
	// key=value and JSON "key":"value" pairs whose key looks secret. A bare
	// "key: text" is left alone so messages like "invalid id token: audience"
	// keep their meaning.
	reSecretPair = regexp.MustCompile(`(?i)\b((?:access_|refresh_|id_|link_)?token|password|passwd|secret|api[_-]?key|authorization|cookie|code|otp|ticket|dsn)(\s*=\s*|"\s*:\s*"?)([^\s"'&,;]+)`)
	// International (+90 555 123 45 67) and Turkish mobile (0555 123 45 67) numbers.
	rePhoneIntl = regexp.MustCompile(`\+\d{1,3}[\s\-.]?\(?\d{2,4}\)?(?:[\s\-.]?\d{2,4}){2,4}`)
	rePhoneTR   = regexp.MustCompile(`\b0?5\d{2}[\s\-.]?\d{3}[\s\-.]?\d{2}[\s\-.]?\d{2}\b`)
)

// sensitiveKeyParts mark map keys whose values are never sent.
var sensitiveKeyParts = []string{
	"password", "passwd", "secret", "token", "authorization", "cookie",
	"email", "e_mail", "phone", "gsm", "msisdn", "dsn", "api_key", "apikey",
	"private_key", "otp", "totp", "ticket", "session_token", "credential",
}

// allowedHeaders are the only request headers forwarded to the tracker.
var allowedHeaders = map[string]bool{
	"User-Agent":     true,
	"Content-Type":   true,
	"Content-Length": true,
	"Accept":         true,
	"X-Request-Id":   true,
}

// ScrubString removes emails, phone numbers, bearer/JWT tokens and
// key=value secrets from free text (error messages, log attributes).
func ScrubString(s string) string {
	if s == "" {
		return s
	}
	s = rePush.ReplaceAllString(s, "[push-token]")
	s = reJWT.ReplaceAllString(s, "[token]")
	s = reBearer.ReplaceAllString(s, "$1 [token]")
	s = reSecretPair.ReplaceAllString(s, "$1$2"+redacted)
	s = reEmail.ReplaceAllString(s, "[email]")
	s = rePhoneIntl.ReplaceAllString(s, "[phone]")
	s = rePhoneTR.ReplaceAllString(s, "[phone]")
	return s
}

// IsSensitiveKey reports whether a field name looks like it carries a secret
// or personal data.
func IsSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

// ScrubValue scrubs a log/extra value recursively. Sensitive keys are dropped
// to a placeholder; strings are passed through ScrubString.
func ScrubValue(key string, v any) any {
	if key != "" && IsSensitiveKey(key) {
		return redacted
	}
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return ScrubString(t)
	case error:
		return ScrubString(t.Error())
	case fmt.Stringer:
		return ScrubString(t.String())
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = ScrubValue(k, val)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, val := range t {
			if IsSensitiveKey(k) {
				out[k] = redacted
				continue
			}
			out[k] = ScrubString(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = ScrubValue("", val)
		}
		return out
	case []string:
		out := make([]string, len(t))
		for i, val := range t {
			out[i] = ScrubString(val)
		}
		return out
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return t
	default:
		return ScrubString(fmt.Sprint(t))
	}
}

func scrubRequest(req *sentry.Request) {
	if req == nil {
		return
	}
	req.Cookies = ""
	req.Data = ""
	req.QueryString = ""
	req.Env = nil
	if req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil {
			u.RawQuery = ""
			u.Fragment = ""
			u.User = nil
			req.URL = u.String()
		} else {
			req.URL = ""
		}
	}
	if len(req.Headers) > 0 {
		kept := make(map[string]string, len(req.Headers))
		for k, v := range req.Headers {
			canon := canonicalHeader(k)
			if allowedHeaders[canon] {
				kept[canon] = ScrubString(v)
			}
		}
		req.Headers = kept
	}
}

func canonicalHeader(k string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(k)), "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}

func scrubMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return m
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = ScrubValue(k, v)
	}
	return out
}

// scrubEvent is the BeforeSend / BeforeSendTransaction hook: it keeps only the
// internal user id, drops request bodies, cookies, query strings and
// non-allowlisted headers, and scrubs every free-text field.
func scrubEvent(event *sentry.Event) *sentry.Event {
	if event == nil {
		return nil
	}
	event.User = sentry.User{ID: event.User.ID}
	scrubRequest(event.Request)
	event.Message = ScrubString(event.Message)
	event.Transaction = ScrubString(event.Transaction)
	for i := range event.Exception {
		event.Exception[i].Value = ScrubString(event.Exception[i].Value)
	}
	for k, v := range event.Tags {
		if IsSensitiveKey(k) {
			event.Tags[k] = redacted
			continue
		}
		event.Tags[k] = ScrubString(v)
	}
	for name, c := range event.Contexts {
		event.Contexts[name] = scrubMap(c)
	}
	for _, b := range event.Breadcrumbs {
		if b == nil {
			continue
		}
		b.Message = ScrubString(b.Message)
		b.Data = scrubMap(b.Data)
	}
	for _, sp := range event.Spans {
		if sp == nil {
			continue
		}
		sp.Description = ScrubString(sp.Description)
		sp.Data = scrubMap(sp.Data)
	}
	return event
}

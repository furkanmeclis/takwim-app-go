// Package pushtext holds the lock-screen-safe texts of mobile push
// notifications. In-app titles/bodies may carry personal data; a push is
// visible on a locked screen, so it only ever uses the fixed texts below plus
// a small whitelist of harmless variables. Add a catalog entry per
// notification kind (template code) that should get a dedicated push text.
package pushtext

import (
	"regexp"
	"strings"
)

// SafeVars are the only template variables a push text may contain. They are
// copied into the in-app notification payload under "push_vars".
var SafeVars = []string{"resource", "succeeded"}

// Text is one localized push.
type Text struct {
	Title string
	Body  string
}

type pair struct{ tr, en Text }

var catalog = map[string]pair{
	"exports.ready": {
		tr: Text{"Dışa aktarım hazır", "{{resource}} dışa aktarımınız indirilmeye hazır."},
		en: Text{"Export ready", "Your {{resource}} export is ready to download."},
	},
	"imports.applied": {
		tr: Text{"İçe aktarım tamamlandı", "{{resource}} içe aktarımı tamamlandı."},
		en: Text{"Import finished", "The {{resource}} import has finished."},
	},
	"bulk.completed": {
		tr: Text{"Toplu işlem tamamlandı", "{{succeeded}} kayıt üzerinde toplu işlem tamamlandı."},
		en: Text{"Bulk action finished", "The bulk action on {{succeeded}} records has finished."},
	},
	"bulk.failed": {
		tr: Text{"Toplu işlem başarısız", "Bir toplu işlem tamamlanamadı."},
		en: Text{"Bulk action failed", "A bulk action could not be completed."},
	},
}

var fallback = pair{
	tr: Text{"Bildirim", "Yeni bir bildiriminiz var."},
	en: Text{"Notification", "You have a new notification."},
}

var placeholderRE = regexp.MustCompile(`\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}`)

// For renders the push for a notification kind (e.g. "exports.ready") in
// locale ("tr" | "en"; anything else → "tr"). Unknown kinds get a generic
// text. Only SafeVars are substituted; a text whose variables are missing
// falls back to the kind-less generic body so no "{{…}}" or empty gap leaks.
func For(kind, locale string, vars map[string]string) Text {
	p, ok := catalog[kind]
	if !ok {
		p = fallback
	}
	t := p.tr
	fb := fallback.tr
	if strings.EqualFold(strings.TrimSpace(locale), "en") {
		t = p.en
		fb = fallback.en
	}
	safe := map[string]string{}
	for _, k := range SafeVars {
		if v := strings.TrimSpace(vars[k]); v != "" {
			safe[k] = v
		}
	}
	title := render(t.Title, safe)
	body := render(t.Body, safe)
	if title == "" {
		title = fb.Title
	}
	if body == "" {
		body = fb.Body
	}
	return Text{Title: title, Body: body}
}

// render substitutes vars; returns "" when any placeholder has no value.
func render(tpl string, vars map[string]string) string {
	missing := false
	out := placeholderRE.ReplaceAllStringFunc(tpl, func(m string) string {
		v := vars[strings.ToLower(placeholderRE.FindStringSubmatch(m)[1])]
		if v == "" {
			missing = true
		}
		return v
	})
	if missing {
		return ""
	}
	return strings.TrimSpace(out)
}

// Known reports whether kind has a dedicated text.
func Known(kind string) bool {
	_, ok := catalog[kind]
	return ok
}

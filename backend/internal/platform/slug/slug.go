package slug

import (
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// turkishFold maps Turkish (and common circumflex) letters to ASCII so
// "Güneş Oto Kiralama" becomes "gunes-oto-kiralama" instead of losing them.
var turkishFold = strings.NewReplacer(
	"ç", "c", "ğ", "g", "ı", "i", "i̇", "i", "ö", "o", "ş", "s", "ü", "u",
	"â", "a", "î", "i", "û", "u",
	"Ç", "c", "Ğ", "g", "I", "i", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u",
	"Â", "a", "Î", "i", "Û", "u",
)

// FromName builds a URL slug from a display name: Turkish letters are
// folded to ASCII, everything else outside [a-z0-9] becomes "-".
func FromName(name string) string {
	name = strings.ToLower(turkishFold.Replace(strings.TrimSpace(name)))
	if name == "" {
		return "item"
	}
	slug := nonSlug.ReplaceAllString(name, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "item"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}

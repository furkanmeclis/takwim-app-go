package usecase

import (
	"strconv"
	"strings"
	"time"
)

// maxSlugLen is the column limit of branches.slug.
const maxSlugLen = 64

var turkishFold = strings.NewReplacer(
	"ç", "c", "Ç", "c", "ğ", "g", "Ğ", "g", "ı", "i", "I", "i", "İ", "i",
	"ö", "o", "Ö", "o", "ş", "s", "Ş", "s", "ü", "u", "Ü", "u",
)

// Slugify turns a branch name into a URL slug: Turkish letters folded to
// ASCII, everything else outside [a-z0-9] collapsed to single dashes.
func Slugify(v string) string {
	v = strings.ToLower(turkishFold.Replace(v))
	var b strings.Builder
	dash := false
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > maxSlugLen {
		out = strings.TrimRight(out[:maxSlugLen], "-")
	}
	return out
}

// withSuffix appends -n, keeping the result within the column limit.
func withSuffix(base string, n int) string {
	suffix := "-" + strconv.Itoa(n)
	if len(base)+len(suffix) > maxSlugLen {
		base = strings.TrimRight(base[:maxSlugLen-len(suffix)], "-")
	}
	return base + suffix
}

// validTimezone reports whether v is a loadable IANA zone name.
func validTimezone(v string) bool {
	if v == "" || v == "Local" || len(v) > 64 {
		return false
	}
	_, err := time.LoadLocation(v)
	return err == nil
}

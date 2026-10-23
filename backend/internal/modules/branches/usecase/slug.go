package usecase

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/slug"
)

// maxSlugLen is the column limit of branches.slug.
const maxSlugLen = 64

// Slugify turns a branch name into a URL slug (slug.FromName: Turkish
// letters folded to ASCII, other characters collapsed to dashes) capped at
// the column length. A name without letters or digits gives "".
func Slugify(v string) string {
	if !strings.ContainsFunc(v, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return ""
	}
	out := slug.FromName(v)
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

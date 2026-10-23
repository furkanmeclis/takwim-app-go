package qrlogin

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

// Location is the approximate place of the browser that asked for a QR.
type Location struct {
	CountryCode string `json:"country_code,omitempty"`
	Country     string `json:"country,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`
	// Source is "edge" (trusted proxy headers), "geoip" (GeoLite2 DB) or "".
	Source string `json:"source,omitempty"`
}

// Empty reports whether no location is known.
func (l Location) Empty() bool {
	return l.CountryCode == "" && l.Country == "" && l.City == ""
}

// Locator resolves a client IP to an approximate location. Order:
//  1. edge geo headers (Cloudflare CF-IPCountry / CF-IPCity / CF-Region),
//     only when TrustHeaders is on (otherwise a phisher could fake "your city");
//  2. a MaxMind GeoLite2-City database when configured;
//  3. nothing (the app shows the IP only).
type Locator struct {
	trustHeaders bool
	db           *geoip2.Reader
}

// NewLocator opens the optional GeoLite2 DB (dbPath may be empty).
func NewLocator(dbPath string, trustHeaders bool) (*Locator, error) {
	l := &Locator{trustHeaders: trustHeaders}
	if strings.TrimSpace(dbPath) == "" {
		return l, nil
	}
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return l, fmt.Errorf("qrlogin: open geoip db: %w", err)
	}
	l.db = db
	return l, nil
}

// Close releases the GeoIP database.
func (l *Locator) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

// Locate returns the best-effort location for r / ip.
func (l *Locator) Locate(r *http.Request, ip string) Location {
	if l == nil {
		return Location{}
	}
	if l.trustHeaders {
		if loc := fromEdgeHeaders(r.Header); !loc.Empty() {
			return loc
		}
	}
	if l.db != nil {
		parsed := net.ParseIP(strings.TrimSpace(ip))
		if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() {
			return Location{}
		}
		rec, err := l.db.City(parsed)
		if err != nil || rec == nil {
			return Location{}
		}
		loc := Location{
			CountryCode: rec.Country.IsoCode,
			Country:     rec.Country.Names["en"],
			City:        rec.City.Names["en"],
			Source:      "geoip",
		}
		if len(rec.Subdivisions) > 0 {
			loc.Region = rec.Subdivisions[0].Names["en"]
		}
		if !loc.Empty() {
			return loc
		}
	}
	return Location{}
}

func fromEdgeHeaders(h http.Header) Location {
	country := strings.ToUpper(strings.TrimSpace(h.Get("CF-IPCountry")))
	// XX = unknown, T1 = Tor.
	if country == "XX" || country == "T1" || len(country) != 2 {
		country = ""
	}
	loc := Location{
		CountryCode: country,
		City:        headerText(h.Get("CF-IPCity")),
		Region:      headerText(h.Get("CF-Region")),
		Source:      "edge",
	}
	if loc.Empty() {
		return Location{}
	}
	return loc
}

// headerText decodes percent-encoded UTF-8 header values and bounds length.
func headerText(v string) string {
	v = strings.TrimSpace(v)
	if dec, err := url.QueryUnescape(v); err == nil {
		v = dec
	}
	if len(v) > 80 {
		v = v[:80]
	}
	return v
}

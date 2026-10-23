package qrlogin

import (
	"regexp"
	"strings"
)

// ClientInfo is a coarse, display-only description of the browser that
// requested the QR. It is parsed from the (client-controlled) User-Agent, so
// it is a hint for the approving user, never a security signal.
type ClientInfo struct {
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	DeviceType     string `json:"device_type"`
	UserAgent      string `json:"user_agent"`
}

var browserRules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Edge", regexp.MustCompile(`Edg(?:e|A|iOS)?/(\d+)`)},
	{"Opera", regexp.MustCompile(`(?:OPR|Opera)/(\d+)`)},
	{"Samsung Internet", regexp.MustCompile(`SamsungBrowser/(\d+)`)},
	{"Yandex", regexp.MustCompile(`YaBrowser/(\d+)`)},
	{"Firefox", regexp.MustCompile(`(?:Firefox|FxiOS)/(\d+)`)},
	{"Chrome", regexp.MustCompile(`(?:Chrome|CriOS)/(\d+)`)},
	{"Safari", regexp.MustCompile(`Version/(\d+)[.\d]* (?:Mobile/\S+ )?Safari/`)},
}

const maxUserAgentLen = 512

// ParseUserAgent extracts browser, OS and device type.
func ParseUserAgent(ua string) ClientInfo {
	ua = strings.TrimSpace(ua)
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	info := ClientInfo{UserAgent: ua, DeviceType: "desktop"}
	for _, rule := range browserRules {
		if m := rule.re.FindStringSubmatch(ua); m != nil {
			info.Browser = rule.name
			info.BrowserVersion = m[1]
			break
		}
	}
	switch {
	case strings.Contains(ua, "iPad"):
		info.OS, info.DeviceType = "iPadOS", "tablet"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPod"):
		info.OS, info.DeviceType = "iOS", "mobile"
	case strings.Contains(ua, "Android"):
		info.OS = "Android"
		if strings.Contains(ua, "Mobile") {
			info.DeviceType = "mobile"
		} else {
			info.DeviceType = "tablet"
		}
	case strings.Contains(ua, "CrOS"):
		info.OS = "ChromeOS"
	case strings.Contains(ua, "Windows"):
		info.OS = "Windows"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		info.OS = "macOS"
	case strings.Contains(ua, "Linux"):
		info.OS = "Linux"
	}
	return info
}

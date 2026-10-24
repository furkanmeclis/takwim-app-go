package storage

import (
	"fmt"
	"net/http"
	"path"
	"strings"
)

// MaxLogoBytes is the maximum accepted logo upload size (2 MiB).
const MaxLogoBytes = 2 << 20

// MaxCoverBytes is the maximum accepted cover image upload size (5 MiB);
// the cover is a wide photo, so it may be larger than a logo.
const MaxCoverBytes = 5 << 20

var logoMIMEToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

var logoExtToMIME = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
}

// NormalizeLogoMIME returns a canonical logo MIME or empty if unsupported.
func NormalizeLogoMIME(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if _, ok := logoMIMEToExt[ct]; ok {
		return ct
	}
	return ""
}

// LogoExtForMIME returns the object-key extension for an allowed logo MIME.
func LogoExtForMIME(contentType string) (string, error) {
	mime := NormalizeLogoMIME(contentType)
	if mime == "" {
		return "", fmt.Errorf("logo content type must be image/jpeg, image/png, or image/webp")
	}
	return logoMIMEToExt[mime], nil
}

// MIMEFromLogoKey derives Content-Type from a logo object key extension.
func MIMEFromLogoKey(objectKey string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(objectKey), "."))
	if mime, ok := logoExtToMIME[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// DetectLogoMIME sniffs the body prefix and accepts only raster logo types.
// The declared Content-Type is used only when no bytes are available: a
// client-declared image/png must not smuggle HTML/SVG into public storage.
func DetectLogoMIME(declared string, head []byte) (string, error) {
	if len(head) == 0 {
		if mime := NormalizeLogoMIME(declared); mime != "" {
			return mime, nil
		}
		return "", fmt.Errorf("logo content type must be image/jpeg, image/png, or image/webp")
	}
	if mime := NormalizeLogoMIME(http.DetectContentType(head)); mime != "" {
		return mime, nil
	}
	return "", fmt.Errorf("logo content type must be image/jpeg, image/png, or image/webp")
}

// ValidateLogoSize rejects oversized uploads.
func ValidateLogoSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("logo file is required")
	}
	if size > MaxLogoBytes {
		return fmt.Errorf("logo must be at most 2 MiB")
	}
	return nil
}

// ValidateCoverSize rejects oversized cover uploads.
func ValidateCoverSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("cover file is required")
	}
	if size > MaxCoverBytes {
		return fmt.Errorf("cover must be at most 5 MiB")
	}
	return nil
}

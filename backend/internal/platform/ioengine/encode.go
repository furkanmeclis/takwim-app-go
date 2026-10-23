package ioengine

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
)

// LetterheadFromSettings builds letterhead from app_settings row.
func LetterheadFromSettings(s db.AppSetting) Letterhead {
	return Letterhead{
		CompanyName:  s.CompanyName,
		Tagline:      s.Tagline,
		PrimaryColor: s.PrimaryColor,
		Address:      s.Address,
		Phone:        s.Phone,
		Email:        s.Email,
		Website:      s.Website,
		FooterText:   s.FooterText,
		PaperSize:    s.PaperSize,
	}
}

// LoadLetterheadLogo fetches logo bytes when configured.
func LoadLetterheadLogo(ctx context.Context, driver storage.Driver, s db.AppSetting) (Letterhead, error) {
	lh := LetterheadFromSettings(s)
	if !s.LogoObjectKey.Valid || s.LogoObjectKey.String == "" || driver == nil {
		return lh, nil
	}
	rc, _, err := driver.Download(ctx, s.LogoObjectKey.String)
	if err != nil {
		return lh, fmt.Errorf("letterhead logo: %w", err)
	}
	defer func() { _ = rc.Close() }()
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, readErr := rc.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	lh.LogoBytes = buf
	lh.LogoMIME = storage.MIMEFromLogoKey(s.LogoObjectKey.String)
	return lh, nil
}

// EncodeExport selects encoder by format.
func EncodeExport(format ExportFormat, ds Dataset, locale string, lh *Letterhead, title string) ([]byte, error) {
	switch format {
	case ExportPDF:
		return EncodePDF(ds, locale, lh, title)
	case ExportXLSX:
		var head *Letterhead
		if lh != nil {
			head = lh
		}
		return EncodeXLSX(ds, locale, head)
	case ExportCSV:
		return EncodeCSV(ds, locale)
	case ExportJSON:
		return EncodeJSON(ds, locale)
	default:
		return nil, fmt.Errorf("ioengine: unknown export format %q", format)
	}
}

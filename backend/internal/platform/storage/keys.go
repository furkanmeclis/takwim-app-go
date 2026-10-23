package storage

import "fmt"

// AppLogoObjectKey builds app/branding/logo.{ext} for system letterhead.
func AppLogoObjectKey(ext string) string {
	return fmt.Sprintf("app/branding/logo.%s", trimExt(ext))
}

// ExportObjectKey builds exports/{jobUUID}/export.{ext}.
func ExportObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("exports/%s/export.%s", jobUUID, trimExt(ext))
}

// ImportSourceObjectKey builds imports/{jobUUID}/source.{ext}.
func ImportSourceObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("imports/%s/source.%s", jobUUID, trimExt(ext))
}

func trimExt(ext string) string {
	for len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}

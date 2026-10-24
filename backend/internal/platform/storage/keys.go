package storage

import (
	"fmt"
	"strings"
)

// AppLogoObjectKey builds app/branding/logo.{ext} for system letterhead.
func AppLogoObjectKey(ext string) string {
	return fmt.Sprintf("app/branding/logo.%s", trimExt(ext))
}

// OrganizationLogoObjectKey builds
// organizations/{orgUUID}/branding/logo-{versionUUID}.{ext}. Every upload
// gets a new version so the API logo URL changes with it (cache busting).
func OrganizationLogoObjectKey(orgUUID, versionUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/branding/logo-%s.%s", orgUUID, versionUUID, trimExt(ext))
}

// OrganizationCoverObjectKey builds
// organizations/{orgUUID}/branding/cover-{versionUUID}.{ext}: the cover
// image of the business page (TEC-966), versioned like the logo.
func OrganizationCoverObjectKey(orgUUID, versionUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/branding/cover-%s.%s", orgUUID, versionUUID, trimExt(ext))
}

// ExportObjectKey builds exports/{jobUUID}/export.{ext}.
func ExportObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("exports/%s/export.%s", jobUUID, trimExt(ext))
}

// OrganizationsRoot is the common prefix of organization-owned objects.
const OrganizationsRoot = "organizations/"

// OrganizationPrefix is the key prefix of every object an organization owns:
// organizations/{orgUUID}/. Tenant uploads and engine files live under it, so
// an organization's objects can be listed, checked and purged together.
func OrganizationPrefix(orgUUID string) string {
	return OrganizationsRoot + orgUUID + "/"
}

// OrganizationOwnsKey reports whether key lies under the organization's
// prefix. Streaming endpoints check it before serving a key read from a row
// (defence in depth on top of the organization filter of the query).
func OrganizationOwnsKey(key, orgUUID string) bool {
	return orgUUID != "" && strings.HasPrefix(key, OrganizationPrefix(orgUUID)) && !strings.Contains(key, "/../")
}

// OrganizationExportObjectKey builds
// organizations/{orgUUID}/exports/{jobUUID}/export.{ext} for tenant exports.
func OrganizationExportObjectKey(orgUUID, jobUUID, ext string) string {
	return fmt.Sprintf("%sexports/%s/export.%s", OrganizationPrefix(orgUUID), jobUUID, trimExt(ext))
}

// OrganizationImportSourceObjectKey builds
// organizations/{orgUUID}/imports/{jobUUID}/source.{ext} for tenant imports.
func OrganizationImportSourceObjectKey(orgUUID, jobUUID, ext string) string {
	return fmt.Sprintf("%simports/%s/source.%s", OrganizationPrefix(orgUUID), jobUUID, trimExt(ext))
}

// BranchImageObjectKey builds
// organizations/{orgUUID}/branches/{branchUUID}/images/{imageUUID}.{ext} for
// branch gallery photos (served through the API's public media route).
func BranchImageObjectKey(orgUUID, branchUUID, imageUUID, ext string) string {
	return fmt.Sprintf("%sbranches/%s/images/%s.%s", OrganizationPrefix(orgUUID), branchUUID, imageUUID, trimExt(ext))
}

// ServiceImageObjectKey builds
// organizations/{orgUUID}/services/{serviceUUID}/image/{imageUUID}.{ext}.
func ServiceImageObjectKey(orgUUID, serviceUUID, imageUUID, ext string) string {
	return fmt.Sprintf("%sservices/%s/image/%s.%s", OrganizationPrefix(orgUUID), serviceUUID, imageUUID, trimExt(ext))
}

// ProductImageObjectKey builds
// organizations/{orgUUID}/products/{productUUID}/image/{imageUUID}.{ext}.
func ProductImageObjectKey(orgUUID, productUUID, imageUUID, ext string) string {
	return fmt.Sprintf("%sproducts/%s/image/%s.%s", OrganizationPrefix(orgUUID), productUUID, imageUUID, trimExt(ext))
}

// FinanceDocumentObjectKey builds
// organizations/{orgUUID}/finance/{entryUUID}/{documentUUID}.{ext} for the
// receipt / invoice of an income or expense record (private; streamed by
// the API).
func FinanceDocumentObjectKey(orgUUID, entryUUID, documentUUID, ext string) string {
	return fmt.Sprintf("%sfinance/%s/%s.%s", OrganizationPrefix(orgUUID), entryUUID, documentUUID, trimExt(ext))
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

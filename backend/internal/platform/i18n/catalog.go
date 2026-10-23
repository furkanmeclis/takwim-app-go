package i18n

import "strings"

// Locale is a supported BCP-47 short code.
type Locale string

const (
	LocaleTR Locale = "tr"
	LocaleEN Locale = "en"
)

// Normalize returns a supported locale or the fallback.
func Normalize(locale string) Locale {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "en":
		return LocaleEN
	default:
		return LocaleTR
	}
}

var catalogs = map[Locale]map[string]string{
	LocaleTR: trCatalog,
	LocaleEN: enCatalog,
}

// Translate resolves a dotted key for the locale with en fallback.
func Translate(locale Locale, key string) string {
	if v, ok := catalogs[locale][key]; ok && v != "" {
		return v
	}
	if locale != LocaleEN {
		if v, ok := catalogs[LocaleEN][key]; ok {
			return v
		}
	}
	return key
}

var trCatalog = map[string]string{
	"users.uuid": "UUID", "users.email": "E-posta", "users.name": "Ad",
	"users.surname": "Soyad", "users.status": "Durum", "users.created_at": "Oluşturulma",
	"users.locale": "Dil", "users.role_slugs": "Roller", "users.role": "Rol",
	"roles.uuid": "UUID", "roles.name": "Ad", "roles.slug": "Slug",
	"roles.is_system": "Sistem rolü", "roles.created_at": "Oluşturulma",
	"roles.description": "Açıklama", "roles.permission_slugs": "İzinler",
	"notifications.uuid": "UUID", "notifications.channel": "Kanal",
	"notifications.status": "Durum", "notifications.priority": "Öncelik",
	"notifications.title": "Başlık", "notifications.created_at": "Oluşturulma",
	"notifications.read_at": "Okunma", "notifications.unread": "Okunmamış",
	"organizations.uuid": "UUID", "organizations.name": "Ad", "organizations.slug": "Kısa ad",
	"organizations.status": "Durum", "organizations.access_ends_at": "Erişim bitişi",
	"organizations.created_at": "Oluşturulma",
	"activity.action":          "İşlem", "activity.resource": "Kaynak",
	"activity.actor": "Kullanıcı", "activity.created_at": "Tarih",
	"common.yes": "Evet", "common.no": "Hayır",
	"users.status.active": "Aktif", "users.status.disabled": "Pasif", "users.status.pending": "Beklemede",
	"export.document":                     "Dışa aktarma",
	"export.generated_at":                 "Oluşturulma",
	"export.title.platform.users":         "Kullanıcılar dışa aktarma",
	"export.title.platform.roles":         "Roller dışa aktarma",
	"export.title.platform.notifications": "Bildirimler dışa aktarma",
	"export.title.platform.activity":      "Etkinlik günlüğü dışa aktarma",
	"resources.platform.users":            "Kullanıcılar",
	"resources.platform.roles":            "Roller",
	"resources.platform.notifications":    "Bildirimler",
	"resources.platform.activity":         "Etkinlik günlüğü",
	"resources.platform.organizations":    "İşletmeler",
	"export.format.pdf":                   "PDF",
	"export.format.xlsx":                  "Excel",
	"export.format.csv":                   "CSV",
	"export.format.json":                  "JSON",
}

var enCatalog = map[string]string{
	"users.uuid": "UUID", "users.email": "Email", "users.name": "First name",
	"users.surname": "Last name", "users.status": "Status", "users.created_at": "Created",
	"users.locale": "Locale", "users.role_slugs": "Roles", "users.role": "Role",
	"roles.uuid": "UUID", "roles.name": "Name", "roles.slug": "Slug",
	"roles.is_system": "System role", "roles.created_at": "Created",
	"roles.description": "Description", "roles.permission_slugs": "Permissions",
	"notifications.uuid": "UUID", "notifications.channel": "Channel",
	"notifications.status": "Status", "notifications.priority": "Priority",
	"notifications.title": "Title", "notifications.created_at": "Created",
	"notifications.read_at": "Read at", "notifications.unread": "Unread",
	"organizations.uuid": "UUID", "organizations.name": "Name", "organizations.slug": "Slug",
	"organizations.status": "Status", "organizations.access_ends_at": "Access ends",
	"organizations.created_at": "Created",
	"activity.action":          "Action", "activity.resource": "Resource",
	"activity.actor": "User", "activity.created_at": "Date",
	"common.yes": "Yes", "common.no": "No",
	"users.status.active": "Active", "users.status.disabled": "Disabled", "users.status.pending": "Pending",
	"export.document":                     "Export",
	"export.generated_at":                 "Generated",
	"export.title.platform.users":         "Users export",
	"export.title.platform.roles":         "Roles export",
	"export.title.platform.notifications": "Notifications export",
	"export.title.platform.activity":      "Activity log export",
	"resources.platform.users":            "Users",
	"resources.platform.roles":            "Roles",
	"resources.platform.notifications":    "Notifications",
	"resources.platform.activity":         "Activity log",
	"resources.platform.organizations":    "Businesses",
	"export.format.pdf":                   "PDF",
	"export.format.xlsx":                  "Excel",
	"export.format.csv":                   "CSV",
	"export.format.json":                  "JSON",
}

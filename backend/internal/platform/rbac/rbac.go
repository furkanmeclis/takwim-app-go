package rbac

// Role slugs (seeded).
const (
	RoleSuperAdmin = "super_admin"
	RoleCMSUser    = "cms_user"
)

// Permission slugs (seeded).
const (
	PermPlatformUsersRead                 = "platform.users.read"
	PermPlatformUsersWrite                = "platform.users.write"
	PermPlatformUsersExport               = "platform.users.export"
	PermPlatformUsersImport               = "platform.users.import"
	PermPlatformUsersBulkDisable          = "platform.users.bulk.disable"
	PermPlatformUsersBulkEnable           = "platform.users.bulk.enable"
	PermPlatformUsersImpersonate          = "platform.users.impersonate"
	PermPlatformRolesRead                 = "platform.roles.read"
	PermPlatformRolesWrite                = "platform.roles.write"
	PermPlatformRolesExport               = "platform.roles.export"
	PermPlatformRolesImport               = "platform.roles.import"
	PermPlatformRolesBulkDelete           = "platform.roles.bulk.delete"
	PermPlatformOrganizationsRead         = "platform.organizations.read"
	PermPlatformOrganizationsWrite        = "platform.organizations.write"
	PermPlatformBulkRead                  = "platform.bulk.read"
	PermPlatformNotificationsRead         = "platform.notifications.read"
	PermPlatformNotificationsReadAll      = "platform.notifications.read_all"
	PermPlatformNotificationsExport       = "platform.notifications.export"
	PermPlatformSettingsRead              = "platform.settings.read"
	PermPlatformSettingsWrite             = "platform.settings.write"
	PermPlatformActivityRead              = "platform.activity.read"
	PermPlatformImportsRead               = "platform.imports.read"
	PermPlatformExportsRead               = "platform.exports.read"
	PermPlatformStorageRead               = "platform.storage.read"
	PermPlatformStorageWrite              = "platform.storage.write"
	PermPlatformLogsRead                  = "platform.logs.read"
	PermPlatformLogsWrite                 = "platform.logs.write"
	PermPlatformAccessRead                = "platform.access.read"
	PermPlatformAccessWrite               = "platform.access.write"
	PermPlatformIntegrationsGitHubRead    = "platform.integrations.github.read"
	PermPlatformIntegrationsGitHubWrite   = "platform.integrations.github.write"
	PermPlatformIntegrationsGoogleRead    = "platform.integrations.google.read"
	PermPlatformIntegrationsGoogleWrite   = "platform.integrations.google.write"
	PermPlatformIntegrationsFacebookRead  = "platform.integrations.facebook.read"
	PermPlatformIntegrationsFacebookWrite = "platform.integrations.facebook.write"
	PermPlatformIntegrationsAppleRead     = "platform.integrations.apple.read"
	PermPlatformIntegrationsAppleWrite    = "platform.integrations.apple.write"
	PermPlatformAuthSettingsRead          = "platform.auth.settings.read"
	PermPlatformAuthSettingsWrite         = "platform.auth.settings.write"
	PermAuthSession                       = "auth.session"
	PermNotificationsRead                 = "notifications.read"
	PermNotificationsManage               = "notifications.manage"
	PermTenantSettingsRead                = "tenant.settings.read"
	PermTenantSettingsWrite               = "tenant.settings.write"
	PermTenantMembersRead                 = "tenant.members.read"
	PermTenantMembersWrite                = "tenant.members.write"
	PermTenantRolesRead                   = "tenant.roles.read"
	PermTenantRolesWrite                  = "tenant.roles.write"
	PermTenantBranchesRead                = "tenant.branches.read"
	PermTenantBranchesWrite               = "tenant.branches.write"
	PermTenantBranchesExport              = "tenant.branches.export"
	PermTenantBranchesImport              = "tenant.branches.import"
	PermTenantBranchesBulkActivate        = "tenant.branches.bulk.activate"
	PermTenantBranchesBulkDeactivate      = "tenant.branches.bulk.deactivate"
	// Sensitive actions of later tenant modules (TEC-689), separate from the
	// resource read/write slugs so roles can grant them one by one.
	PermTenantPriceOverride    = "tenant.price.override"
	PermTenantDiscountApply    = "tenant.discount.apply"
	PermTenantPaymentCollect   = "tenant.payment.collect"
	PermTenantPaymentRefund    = "tenant.payment.refund"
	PermTenantDamageApprove    = "tenant.damage.approve"
	PermTenantSignatureWaive   = "tenant.signature.waive"
	PermTenantDocumentDownload = "tenant.document.download"
	PermTenantReportExport     = "tenant.report.export"
)

// Tenant template role keys seeded by migrations 000030 and 000032.
const (
	TenantRoleOwner            = "owner"
	TenantRoleAdmin            = "admin"
	TenantRoleMember           = "member"
	TenantRoleMerkezYoneticisi = "merkez_yoneticisi"
	TenantRoleSubeYoneticisi   = "sube_yoneticisi"
	TenantRoleRezervasyonSatis = "rezervasyon_satis"
	TenantRoleMuhasebe         = "muhasebe"
	TenantRoleTeslimIade       = "teslim_iade"
)

// IsSystemRole reports whether slug is a protected system role.
func IsSystemRole(slug string) bool {
	return slug == RoleSuperAdmin
}

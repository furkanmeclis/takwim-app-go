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
	PermPlatformOrganizationsExport       = "platform.organizations.export"
	PermPlatformOrganizationsBulkActivate = "platform.organizations.bulk.activate"
	PermPlatformOrganizationsBulkSuspend  = "platform.organizations.bulk.suspend"
	PermPlatformCatalogRead               = "platform.catalog.read"
	PermPlatformCatalogWrite              = "platform.catalog.write"
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
	PermTenantIntegrationsManage          = "tenant.integrations.manage"
	PermTenantIntegrationsRead            = "tenant.integrations.read"
	// PermTenantActivityRead opens the activity history (TEC-893), limited
	// to the member's branch scope.
	PermTenantActivityRead = "tenant.activity.read"
	// Staff (TEC-890): profiles and branch assignments. Shifts and time
	// off are schedule data (PermTenantSchedulesWrite).
	PermTenantStaffRead           = "tenant.staff.read"
	PermTenantStaffWrite          = "tenant.staff.write"
	PermTenantStaffExport         = "tenant.staff.export"
	PermTenantStaffImport         = "tenant.staff.import"
	PermTenantStaffBulkActivate   = "tenant.staff.bulk.activate"
	PermTenantStaffBulkDeactivate = "tenant.staff.bulk.deactivate"
	// Resources (TEC-891): bookable capacity of a branch (chair, table, area,
	// room, device, bay).
	PermTenantResourcesRead           = "tenant.resources.read"
	PermTenantResourcesWrite          = "tenant.resources.write"
	PermTenantResourcesExport         = "tenant.resources.export"
	PermTenantResourcesImport         = "tenant.resources.import"
	PermTenantResourcesBulkActivate   = "tenant.resources.bulk.activate"
	PermTenantResourcesBulkDeactivate = "tenant.resources.bulk.deactivate"
	// Services (TEC-894): the business-level catalog. "offer" lets a member
	// set price, duration and availability in the branches of their scope.
	PermTenantServicesRead           = "tenant.services.read"
	PermTenantServicesWrite          = "tenant.services.write"
	PermTenantServicesOffer          = "tenant.services.offer"
	PermTenantServicesExport         = "tenant.services.export"
	PermTenantServicesImport         = "tenant.services.import"
	PermTenantServicesBulkActivate   = "tenant.services.bulk.activate"
	PermTenantServicesBulkDeactivate = "tenant.services.bulk.deactivate"
	// Products and stock (TEC-918): business-level catalog, branch offers,
	// stock movements ("stock": receipt, return, count, transfer; "sell":
	// sale) inside the member's branch scope.
	PermTenantProductsRead           = "tenant.products.read"
	PermTenantProductsWrite          = "tenant.products.write"
	PermTenantProductsOffer          = "tenant.products.offer"
	PermTenantProductsStock          = "tenant.products.stock"
	PermTenantProductsSell           = "tenant.products.sell"
	PermTenantProductsExport         = "tenant.products.export"
	PermTenantProductsImport         = "tenant.products.import"
	PermTenantProductsBulkActivate   = "tenant.products.bulk.activate"
	PermTenantProductsBulkDeactivate = "tenant.products.bulk.deactivate"
	// Session packages (TEC-920): read the packages and the customers'
	// packages, manage the definitions, sell a package to a customer.
	PermTenantSessionPackagesRead  = "tenant.session_packages.read"
	PermTenantSessionPackagesWrite = "tenant.session_packages.write"
	PermTenantSessionPackagesSell  = "tenant.session_packages.sell"
	// Class timetable (TEC-970): read the timetable, the sessions and their
	// bookings; manage classes and single sessions.
	PermTenantClassesRead  = "tenant.classes.read"
	PermTenantClassesWrite = "tenant.classes.write"
	// Takwim modules (TEC-889). A ".own" slug limits the action to the
	// member's own records; the full slug covers every record inside the
	// member's branch scope (orgctx.Scope.Reach).
	PermTenantReservationsRead  = "tenant.reservations.read"
	PermTenantReservationsWrite = "tenant.reservations.write"
	PermTenantReservationsOwn   = "tenant.reservations.own"
	// Reservation engines (TEC-963): export and bulk transitions.
	PermTenantReservationsExport     = "tenant.reservations.export"
	PermTenantReservationsBulkAccept = "tenant.reservations.bulk.accept"
	PermTenantReservationsBulkReject = "tenant.reservations.bulk.reject"
	PermTenantReservationsBulkCancel = "tenant.reservations.bulk.cancel"
	PermTenantSchedulesRead          = "tenant.schedules.read"
	PermTenantSchedulesWrite         = "tenant.schedules.write"
	PermTenantSchedulesOwn           = "tenant.schedules.own"
	PermTenantCustomersRead          = "tenant.customers.read"
	PermTenantCustomersWrite         = "tenant.customers.write"
	// Customer engines (TEC-896).
	PermTenantCustomersExport     = "tenant.customers.export"
	PermTenantCustomersImport     = "tenant.customers.import"
	PermTenantCustomersBulkTag    = "tenant.customers.bulk.tag"
	PermTenantCustomersBulkDelete = "tenant.customers.bulk.delete"
	PermTenantFinanceRead         = "tenant.finance.read"
	PermTenantFinanceWrite        = "tenant.finance.write"
	PermTenantEarningsRead        = "tenant.earnings.read"
	PermTenantEarningsOwn         = "tenant.earnings.own"
	// Staff shares (TEC-917): managing the rates and paying shares out.
	PermTenantEarningsManage = "tenant.earnings.manage"
	PermTenantEarningsPay    = "tenant.earnings.pay"
	// Cash and bank accounts (TEC-914): read, post movements / transfers,
	// manage accounts, close a till's day, reopen a closed day.
	PermTenantCashRead         = "tenant.cash.read"
	PermTenantCashWrite        = "tenant.cash.write"
	PermTenantCashManage       = "tenant.cash.manage"
	PermTenantCashClose        = "tenant.cash.close"
	PermTenantCashReopen       = "tenant.cash.reopen"
	PermTenantCashExport       = "tenant.cash.export"
	PermTenantCashBulkArchive  = "tenant.cash.bulk.archive"
	PermTenantCashBulkActivate = "tenant.cash.bulk.activate"
	// Sensitive actions of later tenant modules (TEC-689), separate from the
	// resource read/write slugs so roles can grant them one by one.
	PermTenantPriceOverride    = "tenant.price.override"
	PermTenantDiscountApply    = "tenant.discount.apply"
	PermTenantPaymentCollect   = "tenant.payment.collect"
	PermTenantPaymentRefund    = "tenant.payment.refund"
	PermTenantDocumentDownload = "tenant.document.download"
	PermTenantReportExport     = "tenant.report.export"
)

// Tenant template role keys (migration 000042): İşletme sahibi, Yönetici,
// Şube yöneticisi, Resepsiyon and Çalışan.
const (
	TenantRoleOwner          = "owner"
	TenantRoleAdmin          = "admin"
	TenantRoleSubeYoneticisi = "sube_yoneticisi"
	TenantRoleResepsiyon     = "resepsiyon"
	TenantRoleMember         = "member"
)

// IsSystemRole reports whether slug is a protected system role.
func IsSystemRole(slug string) bool {
	return slug == RoleSuperAdmin
}

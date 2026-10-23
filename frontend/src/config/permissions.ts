/**
 * Permission catalog aligned with platform RBAC seed permissions.
 */
export const Permission = {
  PlatformUsersRead: "platform.users.read",
  PlatformUsersWrite: "platform.users.write",
  PlatformUsersExport: "platform.users.export",
  PlatformUsersImport: "platform.users.import",
  PlatformRolesRead: "platform.roles.read",
  PlatformRolesWrite: "platform.roles.write",
  PlatformRolesExport: "platform.roles.export",
  PlatformRolesImport: "platform.roles.import",
  PlatformOrganizationsRead: "platform.organizations.read",
  PlatformOrganizationsWrite: "platform.organizations.write",
  PlatformOrganizationsExport: "platform.organizations.export",
  PlatformOrganizationsBulkActivate: "platform.organizations.bulk.activate",
  PlatformOrganizationsBulkSuspend: "platform.organizations.bulk.suspend",
  PlatformCatalogRead: "platform.catalog.read",
  PlatformCatalogWrite: "platform.catalog.write",
  PlatformNotificationsRead: "platform.notifications.read",
  PlatformNotificationsReadAll: "platform.notifications.read_all",
  PlatformNotificationsExport: "platform.notifications.export",
  PlatformSettingsRead: "platform.settings.read",
  PlatformSettingsWrite: "platform.settings.write",
  PlatformActivityRead: "platform.activity.read",
  PlatformImportsRead: "platform.imports.read",
  PlatformExportsRead: "platform.exports.read",
  PlatformStorageRead: "platform.storage.read",
  PlatformStorageWrite: "platform.storage.write",
  PlatformLogsRead: "platform.logs.read",
  PlatformLogsWrite: "platform.logs.write",
  PlatformUsersBulkDisable: "platform.users.bulk.disable",
  PlatformUsersBulkEnable: "platform.users.bulk.enable",
  PlatformUsersImpersonate: "platform.users.impersonate",
  PlatformRolesBulkDelete: "platform.roles.bulk.delete",
  PlatformBulkRead: "platform.bulk.read",
  PlatformAccessRead: "platform.access.read",
  PlatformAccessWrite: "platform.access.write",
  PlatformIntegrationsGitHubRead: "platform.integrations.github.read",
  PlatformIntegrationsGitHubWrite: "platform.integrations.github.write",
  PlatformIntegrationsGoogleRead: "platform.integrations.google.read",
  PlatformIntegrationsGoogleWrite: "platform.integrations.google.write",
  PlatformIntegrationsFacebookRead: "platform.integrations.facebook.read",
  PlatformIntegrationsFacebookWrite: "platform.integrations.facebook.write",
  PlatformIntegrationsAppleRead: "platform.integrations.apple.read",
  PlatformIntegrationsAppleWrite: "platform.integrations.apple.write",
  PlatformAuthSettingsRead: "platform.auth.settings.read",
  PlatformAuthSettingsWrite: "platform.auth.settings.write",

  AuthSession: "auth.session",

  NotificationsRead: "notifications.read",
  NotificationsManage: "notifications.manage",

  TenantSettingsRead: "tenant.settings.read",
  TenantSettingsWrite: "tenant.settings.write",
  TenantMembersRead: "tenant.members.read",
  TenantMembersWrite: "tenant.members.write",
  TenantRolesRead: "tenant.roles.read",
  TenantRolesWrite: "tenant.roles.write",
  TenantBranchesRead: "tenant.branches.read",
  TenantBranchesWrite: "tenant.branches.write",
  TenantBranchesExport: "tenant.branches.export",
  TenantBranchesImport: "tenant.branches.import",
  TenantBranchesBulkActivate: "tenant.branches.bulk.activate",
  TenantBranchesBulkDeactivate: "tenant.branches.bulk.deactivate",
  TenantIntegrationsManage: "tenant.integrations.manage",
  TenantIntegrationsRead: "tenant.integrations.read",
  TenantReservationsRead: "tenant.reservations.read",
  TenantReservationsWrite: "tenant.reservations.write",
  TenantReservationsOwn: "tenant.reservations.own",
  TenantSchedulesRead: "tenant.schedules.read",
  TenantSchedulesWrite: "tenant.schedules.write",
  TenantSchedulesOwn: "tenant.schedules.own",
  TenantCustomersRead: "tenant.customers.read",
  TenantCustomersWrite: "tenant.customers.write",
  TenantFinanceRead: "tenant.finance.read",
  TenantFinanceWrite: "tenant.finance.write",
  TenantEarningsRead: "tenant.earnings.read",
  TenantEarningsOwn: "tenant.earnings.own",
  TenantPriceOverride: "tenant.price.override",
  TenantDiscountApply: "tenant.discount.apply",
  TenantPaymentCollect: "tenant.payment.collect",
  TenantPaymentRefund: "tenant.payment.refund",
  TenantDocumentDownload: "tenant.document.download",
  TenantReportExport: "tenant.report.export",
} as const;

export type PermissionSlug = (typeof Permission)[keyof typeof Permission];

export const ALL_PERMISSIONS: readonly PermissionSlug[] = Object.values(
  Permission,
).sort((a, b) => a.localeCompare(b));

const knownPermissionSet = new Set<string>(ALL_PERMISSIONS);

export function isKnownPermission(slug: string): slug is PermissionSlug {
  return knownPermissionSet.has(slug);
}

export const permissions = {
  users: {
    read: Permission.PlatformUsersRead,
    write: Permission.PlatformUsersWrite,
    export: Permission.PlatformUsersExport,
    import: Permission.PlatformUsersImport,
    bulkDisable: Permission.PlatformUsersBulkDisable,
    bulkEnable: Permission.PlatformUsersBulkEnable,
    impersonate: Permission.PlatformUsersImpersonate,
  },
  roles: {
    read: Permission.PlatformRolesRead,
    write: Permission.PlatformRolesWrite,
    export: Permission.PlatformRolesExport,
    import: Permission.PlatformRolesImport,
    bulkDelete: Permission.PlatformRolesBulkDelete,
  },
  organizations: {
    read: Permission.PlatformOrganizationsRead,
    write: Permission.PlatformOrganizationsWrite,
    export: Permission.PlatformOrganizationsExport,
    bulkActivate: Permission.PlatformOrganizationsBulkActivate,
    bulkSuspend: Permission.PlatformOrganizationsBulkSuspend,
  },
  notifications: {
    read: Permission.NotificationsRead,
    manage: Permission.NotificationsManage,
    platformRead: Permission.PlatformNotificationsRead,
    platformReadAll: Permission.PlatformNotificationsReadAll,
    export: Permission.PlatformNotificationsExport,
  },
  settings: {
    read: Permission.PlatformSettingsRead,
    write: Permission.PlatformSettingsWrite,
  },
  activity: {
    read: Permission.PlatformActivityRead,
  },
  imports: {
    read: Permission.PlatformImportsRead,
  },
  exports: {
    read: Permission.PlatformExportsRead,
  },
  storage: {
    read: Permission.PlatformStorageRead,
    write: Permission.PlatformStorageWrite,
  },
  logs: {
    read: Permission.PlatformLogsRead,
    write: Permission.PlatformLogsWrite,
  },
  bulk: {
    read: Permission.PlatformBulkRead,
  },
  access: {
    read: Permission.PlatformAccessRead,
    write: Permission.PlatformAccessWrite,
  },
  integrations: {
    github: {
      read: Permission.PlatformIntegrationsGitHubRead,
      write: Permission.PlatformIntegrationsGitHubWrite,
    },
    google: {
      read: Permission.PlatformIntegrationsGoogleRead,
      write: Permission.PlatformIntegrationsGoogleWrite,
    },
    facebook: {
      read: Permission.PlatformIntegrationsFacebookRead,
      write: Permission.PlatformIntegrationsFacebookWrite,
    },
    apple: {
      read: Permission.PlatformIntegrationsAppleRead,
      write: Permission.PlatformIntegrationsAppleWrite,
    },
  },
  authSettings: {
    read: Permission.PlatformAuthSettingsRead,
    write: Permission.PlatformAuthSettingsWrite,
  },
  auth: {
    session: Permission.AuthSession,
  },
  tenant: {
    settings: {
      read: Permission.TenantSettingsRead,
      write: Permission.TenantSettingsWrite,
    },
    members: {
      read: Permission.TenantMembersRead,
      write: Permission.TenantMembersWrite,
    },
    roles: {
      read: Permission.TenantRolesRead,
      write: Permission.TenantRolesWrite,
    },
    branches: {
      read: Permission.TenantBranchesRead,
      write: Permission.TenantBranchesWrite,
      export: Permission.TenantBranchesExport,
      import: Permission.TenantBranchesImport,
      bulkActivate: Permission.TenantBranchesBulkActivate,
      bulkDeactivate: Permission.TenantBranchesBulkDeactivate,
    },
    integrations: {
      read: Permission.TenantIntegrationsRead,
      manage: Permission.TenantIntegrationsManage,
    },
    /** ".own" = only the member's own records (requirements §4). */
    reservations: {
      read: Permission.TenantReservationsRead,
      write: Permission.TenantReservationsWrite,
      own: Permission.TenantReservationsOwn,
    },
    schedules: {
      read: Permission.TenantSchedulesRead,
      write: Permission.TenantSchedulesWrite,
      own: Permission.TenantSchedulesOwn,
    },
    customers: {
      read: Permission.TenantCustomersRead,
      write: Permission.TenantCustomersWrite,
    },
    finance: {
      read: Permission.TenantFinanceRead,
      write: Permission.TenantFinanceWrite,
    },
    earnings: {
      read: Permission.TenantEarningsRead,
      own: Permission.TenantEarningsOwn,
    },
    price: {
      override: Permission.TenantPriceOverride,
    },
    discount: {
      apply: Permission.TenantDiscountApply,
    },
    payment: {
      collect: Permission.TenantPaymentCollect,
      refund: Permission.TenantPaymentRefund,
    },
    document: {
      download: Permission.TenantDocumentDownload,
    },
    report: {
      export: Permission.TenantReportExport,
    },
  },
} as const;

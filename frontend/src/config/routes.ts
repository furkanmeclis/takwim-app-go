export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
  },
  /**
   * Customer panel (takwim.link/panel): one area for the people who book,
   * across every business they visit. Separate session (phone + code), no
   * staff shell.
   */
  customer: {
    root: "/panel",
    login: "/panel/login",
    /** Single-use panel link from the booking SMS (`/panel/l#token`). */
    link: "/panel/l",
    /** Link to one visit (`/panel/v#token`); opens no panel session. */
    visit: "/panel/v",
    reservation: (uuid: string) => `/panel/reservations/${uuid}`,
    payments: "/panel/payments",
    /** "Paketlerim": session packages across businesses (TEC-984). */
    packages: "/panel/packages",
    package: (uuid: string) => `/panel/packages/${uuid}`,
    profile: "/panel/profile",
  },
  guest: {
    login: "/platform/login",
    register: "/platform/register",
    forgotPassword: "/platform/forgot-password",
    resetPassword: "/platform/reset-password",
    verifyEmail: "/platform/verify-email",
  },
  /**
   * Shell-independent pages for every signed-in user. `/` only routes the
   * user to their home (see `defaultHomeForUser`).
   */
  account: {
    home: "/",
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
    },
    organizations: "/organizations",
    noAccess: "/no-access",
    /** Public staff invitation page (`/v1/invitations/{token}`). */
    invite: (token: string) => `/invite/${encodeURIComponent(token)}`,
  },
  /**
   * Organization (tenant) shell `/t/{slug}`. The slug in the URL picks the
   * organization; API calls carry it as `X-Organization` (see
   * `lib/api/tenant-request.ts`).
   */
  tenant: {
    root: (slug: string) => `/t/${encodeURIComponent(slug)}`,
    home: (slug: string) => `/t/${encodeURIComponent(slug)}`,
    branches: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/branches`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/branches/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/branches/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/branches/${uuid}/edit`,
    },
    staff: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/staff`,
      create: (slug: string) => `/t/${encodeURIComponent(slug)}/staff/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/staff/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/staff/${uuid}/edit`,
    },
    resources: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/resources`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/resources/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/resources/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/resources/${uuid}/edit`,
    },
    services: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/services`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/services/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/services/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/services/${uuid}/edit`,
    },
    serviceBundles: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/service-bundles`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/service-bundles/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/service-bundles/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/service-bundles/${uuid}/edit`,
    },
    products: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/products`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/products/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/products/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/products/${uuid}/edit`,
    },
    stockMovements: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/stock-movements`,
    },
    customers: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/customers`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/customers/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/customers/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/customers/${uuid}/edit`,
    },
    reports: (slug: string) => `/t/${encodeURIComponent(slug)}/reports`,
    calendar: (slug: string) => `/t/${encodeURIComponent(slug)}/calendar`,
    myAgenda: (slug: string) => `/t/${encodeURIComponent(slug)}/my-agenda`,
    myShares: (slug: string) => `/t/${encodeURIComponent(slug)}/my-shares`,
    staffShares: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/staff-shares`,
    finance: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/finance`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/finance/${uuid}`,
    },
    /** Sale lines, customer ledger and daily figures (TEC-973). */
    salesLedger: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/sales-ledger`,
    /** Multi-session packages (TEC-920): sold packages and definitions. */
    packages: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/packages`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/packages/${uuid}`,
    },
    /** Group class timetable (TEC-970): weekly classes and their sessions. */
    classes: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/classes`,
      session: (slug: string, uuid: string, date: string) =>
        `/t/${encodeURIComponent(slug)}/classes/${uuid}/sessions/${date}`,
    },
    cash: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/cash`,
      create: (slug: string) => `/t/${encodeURIComponent(slug)}/cash/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/cash/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/cash/${uuid}/edit`,
      movements: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/cash/movements`,
      daily: (slug: string) => `/t/${encodeURIComponent(slug)}/cash/daily`,
    },
    reservations: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/reservations`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/${uuid}`,
      followUps: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/follow-ups`,
      /** Customer confirmations and their follow-ups (TEC-924). */
      confirmations: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/confirmations`,
    },
    activity: (slug: string) => `/t/${encodeURIComponent(slug)}/activity`,
    exports: {
      /**
       * Downloads a finished export of the organization (the action link of
       * the "export ready" notification; the API needs `X-Organization`).
       */
      download: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/exports/${uuid}/download`,
    },
    settings: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/settings`,
      members: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/members`,
      integrations: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/integrations`,
      notifications: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/notifications`,
      confirmation: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/confirmation`,
      /** Embed code generator of the booking screen (TEC-908). */
      embed: (slug: string) => `/t/${encodeURIComponent(slug)}/settings/embed`,
      plan: (slug: string) => `/t/${encodeURIComponent(slug)}/settings/plan`,
      roles: {
        root: (slug: string) => `/t/${encodeURIComponent(slug)}/settings/roles`,
        create: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/settings/roles/create`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/settings/roles/${uuid}`,
      },
    },
  },
  platform: {
    root: "/platform",
    home: "/platform",
    /**
     * System Settings shell (`/platform/settings/*`). Platform admin pages
     * live here behind one grouped sub-sidebar (`settingsNav`). The legacy
     * top-level URLs redirect permanently via `next.config.ts`.
     */
    settings: {
      root: "/platform/settings",
      system: "/platform/settings/system",
      environment: "/platform/settings/environment",
      general: "/platform/settings/general",
      maintenance: "/platform/settings/maintenance",
      integrations: "/platform/settings/integrations",
      roles: "/platform/settings/roles",
      activity: "/platform/settings/activity",
      logs: "/platform/settings/logs",
      /** Background work and failures (TEC-930). */
      operations: "/platform/settings/operations",
      storage: "/platform/settings/storage",
      access: "/platform/settings/access",
      auth: "/platform/settings/auth",
      /** Customer numbers that changed hands (TEC-972). */
      customerPhones: "/platform/settings/customer-phones",
      imports: "/platform/settings/imports",
      exports: "/platform/settings/exports",
    },
    roles: {
      root: "/platform/settings/roles",
      create: "/platform/settings/roles/create",
      detail: (uuid: string) => `/platform/settings/roles/${uuid}`,
      edit: (uuid: string) => `/platform/settings/roles/${uuid}/edit`,
    },
    organizations: {
      root: "/platform/organizations",
      create: "/platform/organizations/create",
      detail: (uuid: string) => `/platform/organizations/${uuid}`,
      edit: (uuid: string) => `/platform/organizations/${uuid}/edit`,
    },
    users: {
      root: "/platform/users",
      create: "/platform/users/create",
      detail: (uuid: string) => `/platform/users/${uuid}`,
      edit: (uuid: string) => `/platform/users/${uuid}/edit`,
    },
    notifications: {
      root: "/platform/notifications",
      detail: (uuid: string) => `/platform/notifications/${uuid}`,
    },
    imports: {
      root: "/platform/settings/imports",
      detail: (uuid: string) => `/platform/settings/imports/${uuid}`,
    },
    exports: {
      root: "/platform/settings/exports",
      detail: (uuid: string) => `/platform/settings/exports/${uuid}`,
    },
    access: {
      root: "/platform/settings/access",
    },
    authSettings: {
      root: "/platform/settings/auth",
    },
    integrations: {
      root: "/platform/settings/integrations",
      github: "/platform/settings/integrations?tab=github",
      google: "/platform/settings/integrations?tab=google",
      facebook: "/platform/settings/integrations?tab=facebook",
      apple: "/platform/settings/integrations?tab=apple",
    },
    activity: {
      root: "/platform/settings/activity",
    },
    logs: {
      root: "/platform/settings/logs",
    },
    storage: {
      root: "/platform/settings/storage",
    },
    profile: {
      root: "/platform/profile",
      password: "/platform/profile/password",
      preferences: "/platform/profile/preferences",
    },
  },
  errors: {
    unauthorized: "/unauthorized",
    forbidden: "/forbidden",
  },
} as const;

type NestedRoutes<T> = T extends string
  ? T
  : T extends (...args: never[]) => infer R
    ? R extends string
      ? R
      : never
    : T extends Record<string, unknown>
      ? NestedRoutes<T[keyof T]>
      : never;

export type AppRoute = NestedRoutes<typeof routes>;

export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
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
      storage: "/platform/settings/storage",
      access: "/platform/settings/access",
      auth: "/platform/settings/auth",
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

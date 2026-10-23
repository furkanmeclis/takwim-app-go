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
  cms: {
    root: "/",
    home: "/",
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
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

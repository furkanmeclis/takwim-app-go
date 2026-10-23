import type { components } from "@/generated/api";

export type PublicUser = components["schemas"]["PublicUser"];
export type LoginRequest = components["schemas"]["LoginRequest"];
export type Me = components["schemas"]["Me"];
export type Tokens = components["schemas"]["Tokens"];
export type MeOrganization = components["schemas"]["MeOrganization"];

export type RoleSummary = {
  uuid: string;
  name: string;
  slug: string;
  description?: string | null;
  is_system?: boolean;
};

/** Membership of the signed-in user (from `/v1/auth/me` `organizations[]`). */
export type AuthOrganization = {
  uuid: string;
  slug: string;
  name: string;
  logoUrl?: string;
  status: "active" | "suspended";
  accessEndsAt: string | null;
  role: { key: string; name: string };
  /** `tenant.*` permissions granted in this organization only. */
  permissions: string[];
};

/** Client session identity — profile + RBAC. */
export type AuthUser = {
  uuid: string;
  email: string;
  name: string;
  surname: string;
  fullName: string;
  status: string;
  locale: string;
  isSuperAdmin: boolean;
  emailVerified: boolean;
  permissions: string[];
  roles: string[];
  /** Organizations the user belongs to; empty when tenancy is off. */
  organizations: AuthOrganization[];
  realtimeUserChannel?: string;
  realtimeEnabled?: boolean;
  impersonation?: {
    uuid: string;
    email: string;
    name: string;
    surname: string;
    fullName: string;
  };
};

export function mapMeOrganization(org: MeOrganization): AuthOrganization {
  return {
    uuid: org.uuid,
    slug: org.slug,
    name: org.name,
    logoUrl: org.logo_url || undefined,
    status: org.status,
    accessEndsAt: org.access_ends_at ?? null,
    role: { key: org.role.key, name: org.role.name },
    permissions: org.permissions ?? [],
  };
}

export function mapMeToAuthUser(me: Me): AuthUser {
  const { user, roles, permissions, organizations, realtime, impersonation } =
    me;
  const locale =
    typeof user.locale === "string" && user.locale ? user.locale : "tr";
  const mapped: AuthUser = {
    uuid: user.uuid,
    email: user.email,
    name: user.name,
    surname: user.surname,
    fullName: `${user.name} ${user.surname}`.trim(),
    status: user.status,
    locale,
    isSuperAdmin: Boolean(user.is_super_admin),
    emailVerified: Boolean(user.email_verified),
    permissions: permissions ?? [],
    roles: roles ?? [],
    organizations: (organizations ?? []).map(mapMeOrganization),
    realtimeUserChannel: realtime?.user_channel,
    realtimeEnabled: realtime?.enabled,
  };
  if (impersonation?.user) {
    const actor = impersonation.user;
    mapped.impersonation = {
      uuid: actor.uuid,
      email: actor.email,
      name: actor.name,
      surname: actor.surname,
      fullName: `${actor.name} ${actor.surname}`.trim(),
    };
  }
  return mapped;
}

export function hasPlatformPermission(
  user: AuthUser | null | undefined,
): boolean {
  if (!user) return false;
  if (user.isSuperAdmin || user.roles.includes("super_admin")) return true;
  return user.permissions.some((p) => p.startsWith("platform."));
}

export function isPlatformUser(user: AuthUser | null | undefined): boolean {
  return hasPlatformPermission(user);
}

export function isCmsUser(user: AuthUser | null | undefined): boolean {
  if (!user) return false;
  return !isPlatformUser(user);
}

export function defaultHomeForUser(user: AuthUser): string {
  if (isPlatformUser(user)) return "/platform";
  return "/";
}

/** Memberships of a (possibly older, cached) user object. */
export function userOrganizations(
  user: AuthUser | null | undefined,
): AuthOrganization[] {
  return user?.organizations ?? [];
}

/** Whether the organization's access window has ended at `now`. */
export function isOrganizationAccessExpired(
  org: Pick<AuthOrganization, "accessEndsAt">,
  now: number = Date.now(),
): boolean {
  if (!org.accessEndsAt) return false;
  const ends = Date.parse(org.accessEndsAt);
  return Number.isFinite(ends) && ends <= now;
}

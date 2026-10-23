"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";

import { isApiError, type ApiError } from "@/lib/api";
import {
  isOrganizationAccessExpired,
  userOrganizations,
  type AuthOrganization,
} from "@/lib/auth/types";
import {
  tenantKeys,
  tenantService,
} from "@/features/tenant/services/tenant.service";
import { useAuth } from "@/providers/auth-provider";
import { PermissionProvider } from "@/providers/permission-provider";

export type TenantOrganization = {
  uuid: string;
  slug: string;
  name: string;
  /** API-owned logo stream (`/v1/organizations/{uuid}/logo?v=…`). */
  logoUrl: string | null;
  status: "active" | "suspended";
  accessEndsAt: string | null;
};

export type TenantScope = {
  /** Organization slug from the `/t/{slug}` route. */
  slug: string;
  organization: TenantOrganization | null;
  /** Primary tenant role in this organization (`owner`, `admin`, `member`, …). */
  role: { key: string; name?: string } | null;
  /** Every tenant role key the member holds here (permissions are their union). */
  roles: string[];
  /** `selected`: branch-bound data is limited to the member's branches. */
  branchScope: "all" | "selected";
  /** `tenant.*` permissions in this organization only. */
  permissions: string[];
  /** The API confirmed the membership for this slug. */
  verified: boolean;
  /**
   * The cached `/me` membership is usable while the API answers (active and
   * inside its access window), so the shell can render optimistically.
   */
  optimistic: boolean;
  isLoading: boolean;
  /** API error of the context request (membership, suspension, …). */
  error: ApiError | null;
  /** The context request failed (any error, including network). */
  failed: boolean;
  refetch: () => void;
};

const TenantContext = createContext<TenantScope | null>(null);

const EMPTY: string[] = [];

function fromMembership(org: AuthOrganization): TenantOrganization {
  return {
    uuid: org.uuid,
    slug: org.slug,
    name: org.name,
    logoUrl: org.logoUrl ?? null,
    status: org.status,
    accessEndsAt: org.accessEndsAt,
  };
}

/** User-level permission prefixes that stay valid inside an organization. */
const ACCOUNT_PERMISSION_PREFIXES = ["auth.", "notifications."];

function isAccountPermission(slug: string) {
  return ACCOUNT_PERMISSION_PREFIXES.some((prefix) => slug.startsWith(prefix));
}

function shouldRetry(failureCount: number, error: unknown) {
  if (isApiError(error) && error.status < 500) return false;
  return failureCount < 2;
}

/**
 * Resolves the organization of the `/t/{slug}` route through
 * `GET /v1/tenant/context` (sent with `X-Organization: {slug}`) and scopes
 * `usePermission()` below it to that organization's permissions.
 */
export function TenantProvider({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const { user, isAuthenticated } = useAuth();

  const membership = useMemo(
    () => userOrganizations(user).find((org) => org.slug === slug) ?? null,
    [slug, user],
  );

  const query = useQuery({
    queryKey: tenantKeys.context(slug),
    queryFn: ({ signal }) => tenantService.getContext(slug, signal),
    enabled: isAuthenticated && Boolean(slug),
    retry: shouldRetry,
    staleTime: 30_000,
  });

  const { data, error, isLoading, refetch } = query;

  const value = useMemo<TenantScope>(() => {
    const apiError = isApiError(error) ? error : null;
    const optimistic =
      membership !== null &&
      membership.status === "active" &&
      !isOrganizationAccessExpired(membership);

    if (data) {
      const roleName =
        membership && membership.role.key === data.role
          ? membership.role.name
          : undefined;
      return {
        slug,
        organization: {
          uuid: data.organization.uuid,
          slug: data.organization.slug,
          name: membership?.name ?? data.organization.name,
          logoUrl: membership?.logoUrl ?? null,
          status: data.organization.status,
          accessEndsAt: data.organization.access_ends_at ?? null,
        },
        role: { key: data.role, name: roleName },
        roles: data.roles?.length ? data.roles : [data.role],
        branchScope: data.branch_scope ?? "all",
        permissions: data.permissions ?? EMPTY,
        verified: true,
        optimistic,
        isLoading: false,
        error: apiError,
        failed: Boolean(apiError),
        refetch: () => void refetch(),
      };
    }

    return {
      slug,
      organization: membership ? fromMembership(membership) : null,
      role: membership ? membership.role : null,
      roles: membership ? [membership.role.key] : EMPTY,
      branchScope: "all",
      permissions: membership?.permissions ?? EMPTY,
      verified: false,
      optimistic,
      isLoading,
      error: apiError,
      failed: Boolean(error),
      refetch: () => void refetch(),
    };
  }, [data, error, isLoading, membership, refetch, slug]);

  const roles = value.roles;

  // Organization permissions plus the user's own account-level ones (inbox,
  // session). Platform permissions never apply inside an organization.
  const globalPermissions = user?.permissions;
  const scopedPermissions = useMemo(() => {
    const account = (globalPermissions ?? EMPTY).filter(isAccountPermission);
    return [...new Set([...value.permissions, ...account])];
  }, [globalPermissions, value.permissions]);

  return (
    <TenantContext.Provider value={value}>
      <PermissionProvider permissions={scopedPermissions} roles={roles}>
        {children}
      </PermissionProvider>
    </TenantContext.Provider>
  );
}

/** Current organization scope; only inside the `/t/{slug}` shell. */
export function useTenant(): TenantScope {
  const ctx = useContext(TenantContext);
  if (!ctx) throw new Error("useTenant must be used within TenantProvider");
  return ctx;
}

/** Current organization scope, or `null` outside the tenant shell. */
export function useOptionalTenant(): TenantScope | null {
  return useContext(TenantContext);
}

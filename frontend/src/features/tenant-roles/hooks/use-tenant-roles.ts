"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { tenantRolesService } from "@/features/tenant-roles/services/tenant-roles.service";
import type {
  ListTenantRolesParams,
  TenantRoleRequest,
} from "@/features/tenant-roles/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantKeys } from "@/features/tenant/services/tenant.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const tenantRolesKeys = {
  all: (slug: string) => ["tenant", slug, "roles"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...tenantRolesKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...tenantRolesKeys.all(slug), "meta"] as const,
  permissions: (slug: string) =>
    [...tenantRolesKeys.all(slug), "permissions"] as const,
  detail: (slug: string, uuid: string) =>
    [...tenantRolesKeys.all(slug), "detail", uuid] as const,
};

export function useTenantRolesList(params: ListTenantRolesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantRolesKeys.list(slug, params),
    queryFn: () => tenantRolesService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

/** Every role of the organization (role pickers and filters). */
export function useAllTenantRoles(enabled = true) {
  const { slug } = useTenant();
  const params = { limit: 100, offset: 0 };
  return useQuery({
    queryKey: tenantRolesKeys.list(slug, params),
    queryFn: () => tenantRolesService.list(slug, params),
    enabled,
    staleTime: 60_000,
  });
}

export function useTenantRolesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantRolesKeys.meta(slug),
    queryFn: () => tenantRolesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useTenantPermissionCatalog() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantRolesKeys.permissions(slug),
    queryFn: () => tenantRolesService.permissions(slug),
    staleTime: 5 * 60_000,
  });
}

export function useTenantRole(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantRolesKeys.detail(slug, uuid ?? ""),
    queryFn: () => tenantRolesService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

function useInvalidateRoles() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: tenantRolesKeys.all(slug) });
    // Members list role badges and the caller's own permissions may change.
    void queryClient.invalidateQueries({
      queryKey: ["tenant", slug, "members"],
    });
    void queryClient.invalidateQueries({ queryKey: tenantKeys.context(slug) });
  };
}

export function useCreateTenantRole() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRoles();
  return useMutation({
    mutationFn: (body: TenantRoleRequest) =>
      tenantRolesService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("tenant_roles.toast.created"));
      invalidate();
    },
  });
}

export function useUpdateTenantRole() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRoles();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: TenantRoleRequest }) =>
      tenantRolesService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("tenant_roles.toast.updated"));
      invalidate();
    },
  });
}

export function useDeleteTenantRole() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRoles();
  return useMutation({
    mutationFn: (uuid: string) => tenantRolesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("tenant_roles.toast.deleted"));
      invalidate();
    },
  });
}

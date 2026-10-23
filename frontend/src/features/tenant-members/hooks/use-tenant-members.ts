"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { tenantMembersService } from "@/features/tenant-members/services/tenant-members.service";
import type {
  ListTenantMembersParams,
  TenantMemberPatchRequest,
} from "@/features/tenant-members/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantKeys } from "@/features/tenant/services/tenant.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const tenantMembersKeys = {
  all: (slug: string) => ["tenant", slug, "members"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...tenantMembersKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...tenantMembersKeys.all(slug), "meta"] as const,
};

export function useTenantMembersList(params: ListTenantMembersParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantMembersKeys.list(slug, params),
    queryFn: () => tenantMembersService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useTenantMembersMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantMembersKeys.meta(slug),
    queryFn: () => tenantMembersService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

function useInvalidateMembers() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({
      queryKey: tenantMembersKeys.all(slug),
    });
    // Member counts of roles, and the caller's own scope when they edit
    // themselves.
    void queryClient.invalidateQueries({ queryKey: ["tenant", slug, "roles"] });
    void queryClient.invalidateQueries({ queryKey: tenantKeys.context(slug) });
  };
}

export function useUpdateTenantMember() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateMembers();
  return useMutation({
    mutationFn: ({
      userUuid,
      body,
    }: {
      userUuid: string;
      body: TenantMemberPatchRequest;
    }) => tenantMembersService.update(slug, userUuid, body),
    onSuccess: () => {
      appToast.success(t("tenant_members.toast.updated"));
      invalidate();
    },
  });
}

export function useRemoveTenantMember() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateMembers();
  return useMutation({
    mutationFn: (userUuid: string) =>
      tenantMembersService.remove(slug, userUuid),
    onSuccess: () => {
      appToast.success(t("tenant_members.toast.removed"));
      invalidate();
    },
  });
}

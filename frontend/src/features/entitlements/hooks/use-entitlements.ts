"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { entitlementsService } from "@/features/entitlements/services/entitlements.service";
import type { SetEntitlementsRequest } from "@/features/entitlements/types";
import { organizationKeys } from "@/features/organizations/hooks/use-organizations";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const entitlementKeys = {
  tenant: (slug: string) => ["tenant", slug, "entitlements"] as const,
  platform: (uuid: string) =>
    [...organizationKeys.detail(uuid), "entitlements"] as const,
};

/** The current organization's plan, limits and usage. */
export function useTenantEntitlements() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: entitlementKeys.tenant(slug),
    queryFn: () => entitlementsService.tenant(slug),
    staleTime: 30_000,
  });
}

export function usePlatformEntitlements(uuid: string) {
  return useQuery({
    queryKey: entitlementKeys.platform(uuid),
    queryFn: () => entitlementsService.platform(uuid),
  });
}

export function useSetPlatformEntitlements(uuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: SetEntitlementsRequest) =>
      entitlementsService.setPlatform(uuid, body),
    onSuccess: (data) => {
      queryClient.setQueryData(entitlementKeys.platform(uuid), data);
      appToast.success(t("entitlements.toast.updated"));
      void queryClient.invalidateQueries({
        queryKey: organizationKeys.detail(uuid),
      });
    },
  });
}

"use client";

import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { reservationsKeys } from "@/features/reservations/hooks/query-keys";
import { tenantSettingsService } from "@/features/tenant-settings/services/tenant-settings.service";
import type {
  ListSettingsVersionsParams,
  OrganizationSettingsSaveRequest,
} from "@/features/tenant-settings/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const tenantSettingsKeys = {
  all: (slug: string) => ["tenant", slug, "settings"] as const,
  current: (slug: string) =>
    [...tenantSettingsKeys.all(slug), "current"] as const,
  versions: (slug: string, params: Record<string, unknown>) =>
    [...tenantSettingsKeys.all(slug), "versions", params] as const,
  version: (slug: string, version: number) =>
    [...tenantSettingsKeys.all(slug), "version", version] as const,
};

export function useTenantSettings() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantSettingsKeys.current(slug),
    queryFn: () => tenantSettingsService.current(slug),
  });
}

export function useSaveTenantSettings() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: OrganizationSettingsSaveRequest) =>
      tenantSettingsService.save(slug, body),
    onSuccess: (saved) => {
      queryClient.setQueryData(tenantSettingsKeys.current(slug), saved);
      void queryClient.invalidateQueries({
        queryKey: tenantSettingsKeys.all(slug),
      });
      // The booking form questions of the panel (TEC-905).
      void queryClient.invalidateQueries({
        queryKey: reservationsKeys.form(slug),
      });
      appToast.success(
        t("tenant_settings.toast.saved", { version: saved.version }),
      );
    },
  });
}

export function useSettingsVersions(params: ListSettingsVersionsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantSettingsKeys.versions(slug, params),
    queryFn: () => tenantSettingsService.versions(slug, params),
    placeholderData: (previous) => previous,
  });
}

/** Two stored versions for the diff view (immutable: cached forever). */
export function useSettingsVersionPair(versions: [number, number] | null) {
  const { slug } = useTenant();
  return useQueries({
    queries: (versions ?? []).map((version) => ({
      queryKey: tenantSettingsKeys.version(slug, version),
      queryFn: () => tenantSettingsService.version(slug, version),
      staleTime: Infinity,
    })),
  });
}

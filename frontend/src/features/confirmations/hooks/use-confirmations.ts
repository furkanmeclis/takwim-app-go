"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { confirmationsService } from "@/features/confirmations/services/confirmations.service";
import type { ConfirmationSaveInput } from "@/features/confirmations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const confirmationKeys = {
  all: (slug: string) => ["tenant", slug, "confirmations"] as const,
  settings: (slug: string) =>
    [...confirmationKeys.all(slug), "settings"] as const,
};

export function useConfirmationSettings(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: confirmationKeys.settings(slug),
    queryFn: () => confirmationsService.settings(slug),
    enabled,
  });
}

export function useSaveConfirmationRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ConfirmationSaveInput) =>
      confirmationsService.save(slug, body),
    meta: { silent: true },
    onSuccess: (data) => {
      queryClient.setQueryData(confirmationKeys.settings(slug), data);
      appToast.success(t("confirmations.toast.saved"));
    },
  });
}

export function useResetConfirmationRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (branchUuid: string | null) =>
      confirmationsService.reset(slug, branchUuid),
    onSuccess: (data) => {
      queryClient.setQueryData(confirmationKeys.settings(slug), data);
      appToast.success(t("confirmations.toast.reset"));
    },
  });
}

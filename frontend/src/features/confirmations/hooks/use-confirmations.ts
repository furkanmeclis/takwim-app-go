"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { confirmationsService } from "@/features/confirmations/services/confirmations.service";
import type {
  ConfirmationConfirmInput,
  ConfirmationFacetParams,
  ConfirmationSaveInput,
  ListConfirmationsParams,
} from "@/features/confirmations/types";
import { reservationsKeys } from "@/features/reservations/hooks/query-keys";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { emitApiError, isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const confirmationKeys = {
  all: (slug: string) => ["tenant", slug, "confirmations"] as const,
  settings: (slug: string) =>
    [...confirmationKeys.all(slug), "settings"] as const,
  /**
   * Under the reservations keys: any change of a visit (cancel, move,
   * answer) refreshes the list, the counts and the alerts too.
   */
  work: (slug: string) =>
    [...reservationsKeys.all(slug), "confirmations"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...confirmationKeys.work(slug), "list", params] as const,
  facets: (slug: string, params: Record<string, unknown>) =>
    [...confirmationKeys.work(slug), "facets", params] as const,
  summary: (slug: string) =>
    [...confirmationKeys.work(slug), "summary"] as const,
  attempts: (slug: string, uuid: string) =>
    [...confirmationKeys.work(slug), "attempts", uuid] as const,
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

/**
 * The confirmation list and its alerts are shown with the add-on and the
 * full reservations read (members of their own calendar only do not get
 * the business-wide list).
 */
export function useConfirmationFollowUpAccess() {
  const { hasModule } = useSector();
  const { can } = usePermission();
  return hasModule("confirmation") && can(permissions.tenant.reservations.read);
}

export function useConfirmationList(params: ListConfirmationsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: confirmationKeys.list(slug, params),
    queryFn: () => confirmationsService.list(slug, params),
    placeholderData: keepPreviousData,
  });
}

export function useConfirmationFacets(params: ConfirmationFacetParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: confirmationKeys.facets(slug, params),
    queryFn: () => confirmationsService.facets(slug, params),
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
}

/**
 * Follow-up counts for the nav badge and the panel alert. Requests and
 * deadlines happen in the worker, so it also refreshes every minute.
 */
export function useConfirmationSummary(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: confirmationKeys.summary(slug),
    queryFn: () => confirmationsService.summary(slug),
    enabled,
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

export function useConfirmationAttempts(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: confirmationKeys.attempts(slug, uuid ?? ""),
    queryFn: () => confirmationsService.attempts(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

const CONFIRM_ERROR_KEYS: Record<string, string> = {
  not_open: "confirmations.phone.errors.not_open",
  stale: "reservations.errors.stale",
};

/** Records a confirmation the customer gave on the phone. */
export function useConfirmByPhone() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: reservationsKeys.all(slug) });
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: ConfirmationConfirmInput;
    }) => confirmationsService.confirm(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("confirmations.phone.toast"));
      void refresh();
    },
    onError: (error) => {
      void refresh();
      if (!isApiError(error)) return;
      const known = error.details.find(
        (d) => d.code && CONFIRM_ERROR_KEYS[d.code],
      );
      if (known?.code) {
        appToast.error(t(CONFIRM_ERROR_KEYS[known.code]));
        return;
      }
      emitApiError(error);
    },
  });
}

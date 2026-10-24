"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef } from "react";

import { permissions } from "@/config/permissions";
import { realtimeConfig } from "@/config/realtime";
import { confirmationsService } from "@/features/confirmations/services/confirmations.service";
import type {
  ConfirmationConfirmInput,
  ConfirmationFacetParams,
  ConfirmationSaveInput,
  ConfirmationTextsInput,
  ListConfirmationsParams,
} from "@/features/confirmations/types";
import { RESERVATIONS_RESOURCE } from "@/features/reservations/constants";
import { reservationsKeys } from "@/features/reservations/hooks/query-keys";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import { useChannel } from "@/hooks/use-realtime";
import { emitApiError, isApiError } from "@/lib/api";
import { parseResourceChanged } from "@/lib/realtime";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const confirmationKeys = {
  all: (slug: string) => ["tenant", slug, "confirmations"] as const,
  settings: (slug: string) =>
    [...confirmationKeys.all(slug), "settings"] as const,
  texts: (slug: string) => [...confirmationKeys.all(slug), "texts"] as const,
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

export function useConfirmationTexts(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: confirmationKeys.texts(slug),
    queryFn: () => confirmationsService.texts(slug),
    enabled,
  });
}

export function useSaveConfirmationTexts() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ConfirmationTextsInput) =>
      confirmationsService.saveTexts(slug, body),
    meta: { silent: true },
    onSuccess: (data) => {
      queryClient.setQueryData(confirmationKeys.texts(slug), data);
      appToast.success(t("confirmations.texts.toast_saved"));
    },
  });
}

export function useResetConfirmationTexts() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => confirmationsService.resetTexts(slug),
    onSuccess: (data) => {
      queryClient.setQueryData(confirmationKeys.texts(slug), data);
      appToast.success(t("confirmations.texts.toast_reset"));
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

const SUMMARY_LIVE_DEBOUNCE_MS = 400;

/**
 * Follow-up counts for the nav badge and the panel alert. Requests,
 * deadlines, send failures and delivery reports happen in the worker and
 * are published on the reservations live channel (TEC-981), so the counts
 * follow them right away. A slow refresh stays for what no event marks
 * (a visit whose time came leaves the counts) and for missed events; it is
 * the only refresh when live updates are off.
 */
export function useConfirmationSummary(enabled = true) {
  const { slug, organization } = useTenant();
  const queryClient = useQueryClient();
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );
  useChannel(
    enabled && organization?.uuid
      ? tenantResourceChannel(organization.uuid, RESERVATIONS_RESOURCE)
      : null,
    () => {
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        timer.current = null;
        // Several badges share the query: join a refresh already running.
        void queryClient.invalidateQueries(
          { queryKey: confirmationKeys.summary(slug) },
          { cancelRefetch: false },
        );
      }, SUMMARY_LIVE_DEBOUNCE_MS);
    },
  );
  return useQuery({
    queryKey: confirmationKeys.summary(slug),
    queryFn: () => confirmationsService.summary(slug),
    enabled,
    staleTime: 30_000,
    refetchInterval: realtimeConfig.enabled ? 5 * 60_000 : 60_000,
  });
}

/**
 * Requests and their channel attempts of one visit. A send result or a
 * delivery report of this visit (live channel) refreshes them.
 */
export function useConfirmationAttempts(uuid: string | null) {
  const { slug, organization } = useTenant();
  const { canAny } = usePermission();
  const queryClient = useQueryClient();
  useChannel(
    uuid &&
      organization?.uuid &&
      canAny([
        permissions.tenant.reservations.read,
        permissions.tenant.reservations.own,
      ])
      ? tenantResourceChannel(organization.uuid, RESERVATIONS_RESOURCE)
      : null,
    (message) => {
      const event = parseResourceChanged(message);
      if (!event || !uuid) return;
      if (event.truncated || event.uuids.includes(uuid)) {
        void queryClient.invalidateQueries(
          { queryKey: confirmationKeys.attempts(slug, uuid) },
          { cancelRefetch: false },
        );
      }
    },
  );
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

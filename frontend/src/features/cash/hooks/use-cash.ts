"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { cashKeys } from "@/features/cash/hooks/query-keys";
import { cashService } from "@/features/cash/services/cash.service";
import type {
  CashAccountCreateRequest,
  CashAccountPatchRequest,
  CashClosingCreateRequest,
  CashMovementCreateRequest,
  CashTransferRequest,
  ListCashAccountsParams,
  ListCashMovementsParams,
} from "@/features/cash/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Paging = { limit: number; offset: number };

export function useCashAccounts(params: ListCashAccountsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.accountList(slug, params),
    queryFn: () => cashService.listAccounts(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCashAccountsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.accountsMeta(slug),
    queryFn: () => cashService.accountsMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useCashAccountFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.accountFacets(slug),
    queryFn: () => cashService.accountFacets(slug),
    staleTime: 60_000,
  });
}

/** Accounts for pickers (transfer target, book filter). */
export function useCashAccountOptions(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.accountOptions(slug),
    queryFn: () => cashService.accountOptions(slug),
    staleTime: 60_000,
    enabled,
  });
}

export function useCashAccount(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.account(slug, uuid ?? ""),
    queryFn: () => cashService.getAccount(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useCashAccountMovements(
  uuid: string,
  params: ListCashMovementsParams,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.accountMovements(slug, uuid, params),
    queryFn: () => cashService.accountMovements(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useCashClosings(uuid: string, params: Paging, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.closings(slug, uuid, params),
    queryFn: () => cashService.listClosings(slug, uuid, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCashClosingPreview(
  uuid: string,
  date: string,
  enabled: boolean,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.closingPreview(slug, uuid, date),
    queryFn: () => cashService.previewClosing(slug, uuid, date || undefined),
    enabled,
    staleTime: 0,
  });
}

export function useCashAccountActivity(uuid: string, params: Paging) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.activity(slug, uuid, params),
    queryFn: () => cashService.accountActivity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useCashMovements(params: ListCashMovementsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.movementList(slug, params),
    queryFn: () => cashService.listMovements(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCashMovementsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.movementsMeta(slug),
    queryFn: () => cashService.movementsMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useCashMovementFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.movementFacets(slug),
    queryFn: () => cashService.movementFacets(slug),
    staleTime: 60_000,
  });
}

export function useCashDailySummary(params: { date: string; branch?: string }) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.daily(slug, params),
    queryFn: () =>
      cashService.dailySummary(slug, {
        date: params.date || undefined,
        branch: params.branch,
      }),
    placeholderData: (previous) => previous,
  });
}

/** Any write changes balances, books and summaries: refresh the whole module. */
function useInvalidateCash() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: cashKeys.all(slug) });
}

export function useCreateCashAccount() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (body: CashAccountCreateRequest) =>
      cashService.createAccount(slug, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateCashAccount() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CashAccountPatchRequest;
    }) => cashService.updateAccount(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.updated"));
      void invalidate();
    },
  });
}

export function useCreateCashMovement(accountUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (body: CashMovementCreateRequest) =>
      cashService.createMovement(slug, accountUuid, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.movement_created"));
      void invalidate();
    },
  });
}

export function useCashTransfer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (body: CashTransferRequest) => cashService.transfer(slug, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.transferred"));
      void invalidate();
    },
  });
}

export function useReverseCashMovement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      cashService.reverseMovement(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("cash.toast.reversed"));
      void invalidate();
    },
  });
}

export function useCloseCashDay(accountUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (body: CashClosingCreateRequest) =>
      cashService.closeDay(slug, accountUuid, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.closed"));
      void invalidate();
    },
  });
}

export function useReopenCashDay(accountUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({
      closingUuid,
      reason,
    }: {
      closingUuid: string;
      reason: string;
    }) => cashService.reopenDay(slug, accountUuid, closingUuid, reason),
    onSuccess: () => {
      appToast.success(t("cash.toast.reopened"));
      void invalidate();
    },
  });
}

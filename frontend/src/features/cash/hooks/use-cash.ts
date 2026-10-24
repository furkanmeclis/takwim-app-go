"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { cashKeys } from "@/features/cash/hooks/query-keys";
import { cashService } from "@/features/cash/services/cash.service";
import type {
  CashAccountCreateRequest,
  CashAccountPatchRequest,
  CashClosingCreateRequest,
  CashMovementCreateRequest,
  CashStatementCreateRequest,
  CashStatementLineCreateRequest,
  CashStatementLineMatchRequest,
  CashStatementLinePostRequest,
  CashStatementUpdateRequest,
  CashTransferRequest,
  ListCashAccountsParams,
  ListCashMovementsParams,
  ListCashStatementLinesParams,
  ListCashStatementsParams,
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

// ---------------------------------------------------------------- statements

export function useCashStatements(
  params: ListCashStatementsParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.statementList(slug, params),
    queryFn: () => cashService.listStatements(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCashStatement(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.statement(slug, uuid ?? ""),
    queryFn: () => cashService.getStatement(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useCashStatementSummary(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.statementSummary(slug, uuid),
    queryFn: () => cashService.statementSummary(slug, uuid),
    placeholderData: (previous) => previous,
  });
}

export function useCashStatementLines(
  uuid: string,
  params: ListCashStatementLinesParams,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.statementLines(slug, uuid, params),
    queryFn: () => cashService.statementLines(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useCashStatementLineFacets(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.statementLineFacets(slug, uuid),
    queryFn: () => cashService.statementLineFacets(slug, uuid),
    staleTime: 30_000,
  });
}

export function useCashStatementLinesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.linesMeta(slug),
    queryFn: () => cashService.linesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

/** Movements close to a line (amount, date) for the manual match picker. */
export function useCashLineCandidates(lineUuid: string | null, q: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: cashKeys.lineCandidates(slug, lineUuid ?? "", q),
    queryFn: () => cashService.lineCandidates(slug, lineUuid!, q),
    enabled: Boolean(lineUuid),
    staleTime: 0,
  });
}

export function useCreateCashStatement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (body: CashStatementCreateRequest) =>
      cashService.createStatement(slug, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.statement_created"));
      void invalidate();
    },
  });
}

export function useUpdateCashStatement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CashStatementUpdateRequest;
    }) => cashService.updateStatement(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.statement_updated"));
      void invalidate();
    },
  });
}

export function useDeleteCashStatement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (uuid: string) => cashService.deleteStatement(slug, uuid),
    onSuccess: () => {
      appToast.success(t("cash.toast.statement_deleted"));
      void invalidate();
    },
  });
}

export function useAddCashStatementLine(statementUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: (body: CashStatementLineCreateRequest) =>
      cashService.addStatementLine(slug, statementUuid, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.line_added"));
      void invalidate();
    },
  });
}

export function useAutoMatchCashStatement(statementUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: () => cashService.autoMatch(slug, statementUuid),
    onSuccess: (result) => {
      appToast.success(
        t("cash.toast.auto_matched", {
          matched: result.matched,
          pending: result.pending,
        }),
      );
      void invalidate();
    },
  });
}

export function useMatchCashLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({
      lineUuid,
      body,
    }: {
      lineUuid: string;
      body: CashStatementLineMatchRequest;
    }) => cashService.matchLine(slug, lineUuid, body),
    onSuccess: (line) => {
      appToast.success(
        t(
          line.status === "difference"
            ? "cash.toast.line_difference"
            : "cash.toast.line_matched",
        ),
      );
      void invalidate();
    },
  });
}

export function useResetCashLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({ lineUuid, reason }: { lineUuid: string; reason?: string }) =>
      cashService.resetLine(slug, lineUuid, reason),
    onSuccess: () => {
      appToast.success(t("cash.toast.line_reset"));
      void invalidate();
    },
  });
}

export function usePostCashLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({
      lineUuid,
      body,
    }: {
      lineUuid: string;
      body: CashStatementLinePostRequest;
    }) => cashService.postLine(slug, lineUuid, body),
    onSuccess: () => {
      appToast.success(t("cash.toast.line_posted"));
      void invalidate();
    },
  });
}

export function useIgnoreCashLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({ lineUuid, reason }: { lineUuid: string; reason: string }) =>
      cashService.ignoreLine(slug, lineUuid, reason),
    onSuccess: () => {
      appToast.success(t("cash.toast.line_ignored"));
      void invalidate();
    },
  });
}

export function useReverseCashLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCash();
  return useMutation({
    mutationFn: ({ lineUuid, reason }: { lineUuid: string; reason: string }) =>
      cashService.reverseLine(slug, lineUuid, reason),
    onSuccess: () => {
      appToast.success(t("cash.toast.line_reversed"));
      void invalidate();
    },
  });
}

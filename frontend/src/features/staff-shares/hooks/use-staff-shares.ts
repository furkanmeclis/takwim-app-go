"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { cashKeys } from "@/features/cash/hooks/query-keys";
import { staffSharesKeys } from "@/features/staff-shares/hooks/query-keys";
import { staffSharesService } from "@/features/staff-shares/services/staff-shares.service";
import type {
  ListStaffShareLinesParams,
  ListStaffSharePayoutsParams,
  ListStaffShareRulesParams,
  StaffSharePayoutInput,
  StaffSharePeriod,
  StaffShareRuleInput,
} from "@/features/staff-shares/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useStaffSharesSummary(period: StaffSharePeriod) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffSharesKeys.summary(slug, period),
    queryFn: () => staffSharesService.summary(slug, period),
    placeholderData: (previous) => previous,
  });
}

export function useMyStaffShares(period: StaffSharePeriod) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffSharesKeys.mine(slug, period),
    queryFn: () => staffSharesService.mine(slug, period),
    placeholderData: (previous) => previous,
    retry: false,
  });
}

export function useStaffShareRules(
  params: ListStaffShareRulesParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffSharesKeys.rules(slug, params),
    queryFn: () => staffSharesService.listRules(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useStaffShareLines(
  params: ListStaffShareLinesParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffSharesKeys.lines(slug, params),
    queryFn: () => staffSharesService.listLines(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useStaffSharePayouts(
  params: ListStaffSharePayoutsParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffSharesKeys.payouts(slug, params),
    queryFn: () => staffSharesService.listPayouts(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

/** A write changes balances, lists and the cash book: refresh both modules. */
function useInvalidateStaffShares() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: staffSharesKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: cashKeys.all(slug) }),
    ]);
}

export function useSaveStaffShareRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaffShares();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid?: string;
      body: StaffShareRuleInput;
    }) =>
      uuid
        ? staffSharesService.updateRule(slug, uuid, body)
        : staffSharesService.createRule(slug, body),
    onSuccess: (_rule, { uuid }) => {
      appToast.success(
        t(
          uuid
            ? "staff_shares.toast.rule_updated"
            : "staff_shares.toast.rule_created",
        ),
      );
      void invalidate();
    },
  });
}

export function useDeleteStaffShareRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaffShares();
  return useMutation({
    mutationFn: (uuid: string) => staffSharesService.deleteRule(slug, uuid),
    onSuccess: () => {
      appToast.success(t("staff_shares.toast.rule_deleted"));
      void invalidate();
    },
  });
}

export function usePayStaffShare() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaffShares();
  return useMutation({
    mutationFn: (body: StaffSharePayoutInput) =>
      staffSharesService.pay(slug, body),
    onSuccess: () => {
      appToast.success(t("staff_shares.toast.paid"));
      void invalidate();
    },
  });
}

export function useReverseStaffSharePayout() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaffShares();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      staffSharesService.reversePayout(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("staff_shares.toast.payout_reversed"));
      void invalidate();
    },
  });
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { customersKeys } from "@/features/customers/hooks/query-keys";
import { financeKeys } from "@/features/finance/hooks/query-keys";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { reservationsKeys } from "@/features/reservations/hooks/query-keys";
import { salesKeys } from "@/features/sales/hooks/query-keys";
import { salesService } from "@/features/sales/services/sales.service";
import type {
  ListLedgerEntriesParams,
  ListSaleLinesParams,
  SaleLine,
  SaleLineCorrectRequest,
  SalesPeriod,
} from "@/features/sales/types";
import { staffSharesKeys } from "@/features/staff-shares/hooks/query-keys";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { usePermission } from "@/providers/permission-provider";

/** The ledger is readable here: finance module on and `tenant.finance.read`. */
export function useCanReadSales() {
  const { can } = usePermission();
  const hasModule = useNavModules();
  return hasModule("finance") && can(permissions.tenant.finance.read);
}

export function useSaleLines(params: ListSaleLinesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.lineList(slug, params),
    queryFn: () => salesService.listLines(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useSaleLinesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.linesMeta(slug),
    queryFn: () => salesService.linesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

/** One line, seeded with the row the drawer was opened from. */
export function useSaleLine(uuid: string | null, initial?: SaleLine) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.line(slug, uuid ?? ""),
    queryFn: () => salesService.getLine(slug, uuid!),
    enabled: Boolean(uuid),
    placeholderData: initial && initial.uuid === uuid ? initial : undefined,
  });
}

export function useLedgerEntries(params: ListLedgerEntriesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.entryList(slug, params),
    queryFn: () => salesService.listEntries(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useLedgerEntriesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.entriesMeta(slug),
    queryFn: () => salesService.entriesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useSalesSummary(period: SalesPeriod) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.summary(slug, period),
    queryFn: () => salesService.summary(slug, period),
    placeholderData: (previous) => previous,
  });
}

export function useReservationBalance(uuid: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.reservationBalance(slug, uuid),
    queryFn: () => salesService.reservationBalance(slug, uuid),
    enabled: enabled && Boolean(uuid),
  });
}

export function useCustomerBalance(uuid: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: salesKeys.customerBalance(slug, uuid),
    queryFn: () => salesService.customerBalance(slug, uuid),
    enabled: enabled && Boolean(uuid),
  });
}

/**
 * A reversal or correction changes revenue, the customer's balance, the
 * visit's money, the finance list (sale lines are income there) and the
 * staff shares of the line.
 */
function useInvalidateSales() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all(
      [
        salesKeys.all(slug),
        reservationsKeys.all(slug),
        customersKeys.all(slug),
        financeKeys.all(slug),
        staffSharesKeys.all(slug),
      ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
}

export function useReverseSaleLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateSales();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      salesService.reverseLine(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("sales.toast.reversed"));
      void invalidate();
    },
  });
}

export function useCorrectSaleLine() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateSales();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: SaleLineCorrectRequest;
    }) => salesService.correctLine(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("sales.toast.corrected"));
      void invalidate();
    },
  });
}

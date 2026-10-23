"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { addDays, format, parseISO } from "date-fns";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCashDailySummary } from "@/features/cash/hooks/use-cash";
import {
  amountTone,
  kindLabel,
  useBusinessToday,
  useCashAmount,
} from "@/features/cash/lib/format";
import type { CashDailyAccount } from "@/features/cash/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function shift(day: string, by: number) {
  return format(addDays(parseISO(day), by), "yyyy-MM-dd");
}

/**
 * Day summary per account: opening, money in / out (transfers apart), the
 * closing difference and the end-of-day balance, with totals per currency.
 * The branch picked in the header narrows it like every tenant list.
 */
export function CashDailyPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const money = useCashAmount();
  const today = useBusinessToday();
  const [day, setDay] = useState(today);
  const query = useCashDailySummary({ date: day });
  const data = query.data;

  const columns = useMemo(() => {
    const amountCell = (key: keyof CashDailyAccount, tone = false) =>
      function AmountCell({ row }: { row: { original: CashDailyAccount } }) {
        const value = row.original[key] as string;
        return (
          <span
            className={cn("tabular-nums", tone ? amountTone(value) : undefined)}
          >
            {money(value, row.original.currency)}
          </span>
        );
      };
    return [
      createColumn<CashDailyAccount>({
        id: "account",
        accessorFn: (row) => row.account.name,
        labelKey: "cash.daily.columns.account",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="space-y-0.5">
            <span className="font-medium">{row.original.account.name}</span>
            <span className="text-muted-foreground block text-xs">
              {row.original.branch?.name ?? t("cash.head_office")}
            </span>
          </div>
        ),
      }),
      createColumn<CashDailyAccount>({
        id: "kind",
        accessorFn: (row) => row.account.kind ?? "",
        labelKey: "cash.columns.kind",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.account.kind ? (
            <Badge variant="secondary">
              {kindLabel(t, row.original.account.kind)}
            </Badge>
          ) : null,
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "opening_amount",
        labelKey: "cash.daily.columns.opening_amount",
        enableSorting: false,
        cell: amountCell("opening_amount"),
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "money_in",
        labelKey: "cash.daily.columns.money_in",
        enableSorting: false,
        cell: amountCell("money_in"),
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "money_out",
        labelKey: "cash.daily.columns.money_out",
        enableSorting: false,
        cell: amountCell("money_out"),
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "transfer_in",
        labelKey: "cash.daily.columns.transfer_in",
        enableSorting: false,
        cell: amountCell("transfer_in"),
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "transfer_out",
        labelKey: "cash.daily.columns.transfer_out",
        enableSorting: false,
        cell: amountCell("transfer_out"),
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "difference",
        labelKey: "cash.daily.columns.difference",
        enableSorting: false,
        cell: amountCell("difference", true),
      }),
      createColumn<CashDailyAccount>({
        accessorKey: "closing_amount",
        labelKey: "cash.daily.columns.closing_amount",
        enableSorting: false,
        gridSecondary: true,
        cell: amountCell("closing_amount"),
      }),
      createColumn<CashDailyAccount>({
        id: "closed",
        accessorFn: (row) => String(row.closed),
        labelKey: "cash.daily.columns.closed",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.account.kind === "cash" ? (
            <StatusChip
              label={
                row.original.closed
                  ? t("cash.daily.closed")
                  : t("cash.daily.open")
              }
              tone={row.original.closed ? "success" : "warning"}
            />
          ) : (
            "—"
          ),
      }),
    ] as ColumnDef<CashDailyAccount, unknown>[];
  }, [money, t]);

  return (
    <EntityPage
      title={t("cash.daily.title")}
      description={t("cash.daily.description")}
      permission={permissions.tenant.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("cash.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("cash.title"), href: routes.tenant.cash.root(slug) },
        { label: t("cash.daily.title") },
      ]}
      actions={
        <div className="flex items-center gap-2">
          <Button
            type="button"
            size="icon"
            variant="outline"
            aria-label={t("cash.daily.previous_day")}
            onClick={() => setDay((d) => shift(d, -1))}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <DatePicker
            className="w-40"
            value={day}
            onChange={(value) => setDay(value || today)}
          />
          <Button
            type="button"
            size="icon"
            variant="outline"
            aria-label={t("cash.daily.next_day")}
            disabled={day >= today}
            onClick={() => setDay((d) => shift(d, 1))}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      }
    >
      <div className="space-y-6">
        {(data?.totals ?? []).map((totals) => (
          <div key={totals.currency} className="space-y-2">
            {(data?.totals.length ?? 0) > 1 ? (
              <h3 className="text-muted-foreground text-sm font-medium">
                {totals.currency}
              </h3>
            ) : null}
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <StatsCard
                title={t("cash.daily.columns.opening_amount")}
                value={money(totals.opening_amount, totals.currency)}
              />
              <StatsCard
                title={t("cash.daily.columns.money_in")}
                value={money(totals.money_in, totals.currency)}
                hint={t("cash.daily.transfers_hint", {
                  amount: money(totals.transfer_in, totals.currency),
                })}
              />
              <StatsCard
                title={t("cash.daily.columns.money_out")}
                value={money(totals.money_out, totals.currency)}
                hint={t("cash.daily.transfers_hint", {
                  amount: money(totals.transfer_out, totals.currency),
                })}
              />
              <StatsCard
                title={t("cash.daily.columns.closing_amount")}
                value={money(totals.closing_amount, totals.currency)}
                hint={t("cash.daily.difference_hint", {
                  amount: money(totals.difference, totals.currency),
                })}
              />
            </div>
          </div>
        ))}

        <EntitySectionCard
          title={t("cash.daily.accounts_title", {
            date: formatDate(day, undefined, locale),
          })}
          badge={data ? data.accounts.length : undefined}
        >
          <EntityTable
            columns={columns}
            data={data?.accounts ?? []}
            getRowId={(row) => row.account.uuid}
            onRowClick={(row) =>
              router.push(routes.tenant.cash.detail(slug, row.account.uuid))
            }
            isLoading={query.isLoading}
            isError={query.isError}
            onRetry={() => void query.refetch()}
            emptyTitle={t("cash.empty_title")}
            emptyDescription={t("cash.daily.empty_description")}
            manual={{ filtering: false, sorting: false, pagination: false }}
            features={{
              persistKey: "tenant-cash-daily-v1",
              rowSelection: false,
              columnFilters: false,
              facetedFilters: false,
              globalFilter: false,
              sorting: false,
              pagination: false,
              viewMode: true,
            }}
          />
        </EntitySectionCard>
      </div>
    </EntityPage>
  );
}

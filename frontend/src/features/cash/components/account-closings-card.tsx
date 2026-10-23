"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Lock, LockOpen } from "lucide-react";
import { useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import {
  useCashClosings,
  useReopenCashDay,
} from "@/features/cash/hooks/use-cash";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import type { CashAccount, CashClosing } from "@/features/cash/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

function userName(user: CashClosing["closed_by"]) {
  return user ? `${user.name} ${user.surname}`.trim() : "—";
}

/**
 * Daily closings of a till, newest first. Only the latest closed day can be
 * reopened, by a member with `tenant.cash.reopen`, with a reason.
 */
export function AccountClosingsCard({
  account,
  onCloseDay,
}: {
  account: CashAccount;
  onCloseDay: () => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const query = useCashClosings(account.uuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const reopen = useReopenCashDay(account.uuid);
  const [target, setTarget] = useState<CashClosing | null>(null);
  const total = query.data?.total ?? 0;
  const latest = account.last_closed_date;

  const columns = [
    createColumn<CashClosing>({
      accessorKey: "business_date",
      labelKey: "cash.closing.business_date",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">
          {date(row.original.business_date, undefined, locale)}
        </span>
      ),
    }),
    createColumn<CashClosing>({
      accessorKey: "expected_amount",
      labelKey: "cash.closing.expected_amount",
      enableSorting: false,
      cell: ({ row }) =>
        money(row.original.expected_amount, row.original.currency),
    }),
    createColumn<CashClosing>({
      accessorKey: "counted_amount",
      labelKey: "cash.closing.counted_amount",
      enableSorting: false,
      cell: ({ row }) =>
        money(row.original.counted_amount, row.original.currency),
    }),
    createColumn<CashClosing>({
      accessorKey: "difference",
      labelKey: "cash.closing.difference",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <span
          className={cn(
            "font-medium tabular-nums",
            amountTone(row.original.difference),
          )}
        >
          {money(row.original.difference, row.original.currency)}
        </span>
      ),
    }),
    createColumn<CashClosing>({
      accessorKey: "status",
      labelKey: "cash.closing.status",
      enableSorting: false,
      cell: ({ row }) => (
        <StatusChip
          label={t(`cash.closing_status.${row.original.status}`)}
          tone={row.original.status === "closed" ? "success" : "warning"}
        />
      ),
    }),
    createColumn<CashClosing>({
      id: "closed_by",
      accessorFn: (row) => userName(row.closed_by),
      labelKey: "cash.closing.closed_by",
      enableSorting: false,
      cell: ({ row }) => (
        <div className="space-y-0.5 text-sm">
          <span>{userName(row.original.closed_by)}</span>
          <span className="text-muted-foreground block text-xs">
            {datetime(row.original.closed_at, undefined, locale)}
          </span>
        </div>
      ),
    }),
    createColumn<CashClosing>({
      accessorKey: "note",
      labelKey: "cash.closing.note",
      enableSorting: false,
      defaultHidden: true,
      cell: ({ row }) => (
        <span className="text-muted-foreground line-clamp-2 text-sm">
          {row.original.status === "reopened"
            ? t("cash.closing.reopened_note", {
                reason: row.original.reopen_reason,
                name: userName(row.original.reopened_by),
              })
            : row.original.note || "—"}
        </span>
      ),
    }),
    createColumn<CashClosing>({
      id: "actions",
      labelKey: "cash.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) =>
        row.original.status === "closed" &&
        row.original.business_date === latest ? (
          <EntityRowActions
            actions={[
              {
                id: "reopen",
                label: t("cash.actions.reopen"),
                icon: LockOpen,
                permission: permissions.tenant.cash.reopen,
                variant: "destructive",
                onSelect: () => setTarget(row.original),
              },
            ]}
          />
        ) : null,
    }),
  ] as ColumnDef<CashClosing, unknown>[];

  return (
    <EntitySectionCard
      title={t("cash.detail.closings")}
      badge={query.data ? total : undefined}
      actions={
        account.status === "active" ? (
          <PermissionGuard permission={permissions.tenant.cash.close}>
            <Button type="button" size="sm" onClick={onCloseDay}>
              <Lock className="size-4" />
              {t("cash.actions.close_day")}
            </Button>
          </PermissionGuard>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("cash.closing.empty_title")}
        emptyDescription={t("cash.closing.empty_description")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        manual={{ filtering: false, sorting: false, pagination: true }}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          viewMode: true,
          persistKey: "tenant-cash-closings-v1",
        }}
      />
      <ReasonDialog
        open={Boolean(target)}
        title={t("cash.reopen_dialog.title")}
        description={t("cash.reopen_dialog.description", {
          date: target ? date(target.business_date, undefined, locale) : "",
        })}
        confirmLabel={t("cash.actions.reopen")}
        isPending={reopen.isPending}
        onOpenChange={(open) => {
          if (!open) setTarget(null);
        }}
        onConfirm={(reason) =>
          reopen.mutateAsync({ closingUuid: target!.uuid, reason })
        }
      />
    </EntitySectionCard>
  );
}

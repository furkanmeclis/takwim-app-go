"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, FileSpreadsheet, Pencil, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { StatementDialog } from "@/features/cash/components/statement-dialog";
import {
  useCashStatements,
  useDeleteCashStatement,
} from "@/features/cash/hooks/use-cash";
import { useCashAmount } from "@/features/cash/lib/format";
import type { CashAccount, CashStatement } from "@/features/cash/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import type { AppLocale } from "@/config/i18n";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

/** "01.06.2026 – 30.06.2026", or one side when the other is missing. */
export function periodLabel(
  statement: Pick<CashStatement, "period_from" | "period_to">,
  locale: AppLocale,
) {
  const from = statement.period_from
    ? date(statement.period_from, undefined, locale)
    : "";
  const to = statement.period_to
    ? date(statement.period_to, undefined, locale)
    : "";
  if (from && to) return `${from} – ${to}`;
  return from || to || "—";
}

/**
 * Statements (bank statements, provider settlement reports) imported on a
 * bank / online account, newest period first. Each opens its own page where
 * the lines are matched and posted.
 */
export function AccountStatementsCard({ account }: { account: CashAccount }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const money = useCashAmount();
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const query = useCashStatements({
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
    account: account.uuid,
    sort: "-period_to",
  });
  const remove = useDeleteCashStatement();
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<CashStatement | null>(null);
  const [deleting, setDeleting] = useState<CashStatement | null>(null);
  const total = query.data?.total ?? 0;
  const open = (statement: CashStatement) =>
    router.push(
      routes.tenant.cash.statement(slug, account.uuid, statement.uuid),
    );

  const columns = [
    createColumn<CashStatement>({
      accessorKey: "name",
      labelKey: "cash.statements.columns.name",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <div className="min-w-0 space-y-0.5">
          <span className="font-medium">{row.original.name}</span>
          <span className="text-muted-foreground block text-xs">
            {t(`cash.statement_sources.${row.original.source}`)}
          </span>
        </div>
      ),
    }),
    createColumn<CashStatement>({
      id: "period",
      accessorFn: (row) => row.period_to ?? "",
      labelKey: "cash.statements.columns.period",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => periodLabel(row.original, locale),
    }),
    createColumn<CashStatement>({
      accessorKey: "line_count",
      labelKey: "cash.statements.columns.line_count",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="tabular-nums">
          {row.original.open_count
            ? t("cash.statements.open_of_total", {
                open: row.original.open_count,
                total: row.original.line_count,
              })
            : row.original.line_count}
        </span>
      ),
    }),
    createColumn<CashStatement>({
      accessorKey: "closing_balance",
      labelKey: "cash.statements.columns.closing_balance",
      enableSorting: false,
      cell: ({ row }) =>
        row.original.closing_balance ? (
          <span className="tabular-nums">
            {money(row.original.closing_balance, row.original.currency)}
          </span>
        ) : (
          "—"
        ),
    }),
    createColumn<CashStatement>({
      accessorKey: "status",
      labelKey: "cash.statements.columns.status",
      enableSorting: false,
      cell: ({ row }) => {
        const reconciled =
          row.original.status === "open" &&
          row.original.line_count > 0 &&
          row.original.open_count === 0;
        return (
          <StatusChip
            label={
              reconciled
                ? t("cash.statement_status.reconciled")
                : t(`cash.statement_status.${row.original.status}`)
            }
            tone={
              row.original.status === "closed"
                ? "default"
                : reconciled
                  ? "success"
                  : "warning"
            }
          />
        );
      },
    }),
    createColumn<CashStatement>({
      accessorKey: "created_at",
      labelKey: "cash.statements.columns.created_at",
      enableSorting: false,
      defaultHidden: true,
      cell: ({ row }) => date(row.original.created_at, undefined, locale),
    }),
    createColumn<CashStatement>({
      id: "actions",
      labelKey: "cash.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "view",
              label: t("cash.actions.view"),
              icon: Eye,
              onSelect: () => open(row.original),
            },
            ...(row.original.status === "open"
              ? [
                  {
                    id: "edit",
                    label: t("cash.actions.edit"),
                    icon: Pencil,
                    permission: permissions.tenant.cash.import,
                    onSelect: () => setEditing(row.original),
                  },
                  {
                    id: "delete",
                    label: t("cash.actions.delete_statement"),
                    icon: Trash2,
                    permission: permissions.tenant.cash.import,
                    variant: "destructive" as const,
                    separatorBefore: true,
                    onSelect: () => setDeleting(row.original),
                  },
                ]
              : []),
          ]}
        />
      ),
    }),
  ] as ColumnDef<CashStatement, unknown>[];

  return (
    <EntitySectionCard
      title={t("cash.detail.statements")}
      badge={query.data ? total : undefined}
      actions={
        account.status === "active" ? (
          <PermissionGuard permission={permissions.tenant.cash.import}>
            <Button type="button" size="sm" onClick={() => setCreateOpen(true)}>
              <FileSpreadsheet className="size-4" />
              {t("cash.actions.new_statement")}
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
        emptyTitle={t("cash.statements.empty_title")}
        emptyDescription={t(
          account.kind === "online"
            ? "cash.statements.empty_description_provider"
            : "cash.statements.empty_description_bank",
        )}
        onRowClick={open}
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
          persistKey: "tenant-cash-statements-v1",
        }}
      />
      <StatementDialog
        account={account}
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={open}
      />
      <StatementDialog
        account={account}
        statement={editing}
        open={Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        title={t("cash.delete_statement_dialog.title")}
        description={t("cash.delete_statement_dialog.description", {
          name: deleting?.name ?? "",
        })}
        confirmLabel={t("cash.actions.delete_statement")}
        variant="destructive"
        isPending={remove.isPending}
        onConfirm={() => {
          if (!deleting) return;
          remove.mutate(deleting.uuid, { onSuccess: () => setDeleting(null) });
        }}
        onCancel={() => setDeleting(null)}
      />
    </EntitySectionCard>
  );
}

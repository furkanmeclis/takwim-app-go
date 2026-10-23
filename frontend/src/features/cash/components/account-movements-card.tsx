"use client";

import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { useCallback, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useCashMovementColumns } from "@/features/cash/components/cash-movement-columns";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import {
  useCashAccountMovements,
  useReverseCashMovement,
} from "@/features/cash/hooks/use-cash";
import type {
  CashAccount,
  CashDirection,
  CashMovement,
} from "@/features/cash/types";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 20;

/** An account's own book, newest first, with the balance after each row. */
export function AccountMovementsCard({
  account,
  onPost,
}: {
  account: CashAccount;
  onPost: (direction: CashDirection) => void;
}) {
  const { t } = useLocale();
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const query = useCashAccountMovements(account.uuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const reverse = useReverseCashMovement();
  const [target, setTarget] = useState<CashMovement | null>(null);
  const onReverse = useCallback((m: CashMovement) => setTarget(m), []);
  const columns = useCashMovementColumns({
    showAccount: false,
    showBalance: true,
    filters: false,
    onReverse,
  });
  const total = query.data?.total ?? 0;
  const active = account.status === "active";

  return (
    <EntitySectionCard
      title={t("cash.detail.movements")}
      badge={query.data ? total : undefined}
      actions={
        active ? (
          <PermissionGuard permission={permissions.tenant.cash.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => onPost("in")}
              >
                <ArrowDownLeft className="size-4" />
                {t("cash.actions.money_in")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => onPost("out")}
              >
                <ArrowUpRight className="size-4" />
                {t("cash.actions.money_out")}
              </Button>
            </EntityActions>
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
        emptyTitle={t("cash.movements.empty_title")}
        emptyDescription={t("cash.movements.empty_account_description")}
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
          persistKey: "tenant-cash-account-movements-v1",
        }}
      />
      <ReasonDialog
        open={Boolean(target)}
        title={t("cash.reverse_dialog.title")}
        description={t(
          target?.transfer_group
            ? "cash.reverse_dialog.description_transfer"
            : "cash.reverse_dialog.description",
        )}
        confirmLabel={t("cash.actions.reverse")}
        isPending={reverse.isPending}
        onOpenChange={(open) => {
          if (!open) setTarget(null);
        }}
        onConfirm={(reason) =>
          reverse.mutateAsync({ uuid: target!.uuid, reason })
        }
      />
    </EntitySectionCard>
  );
}

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { LogOut } from "lucide-react";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  useCustomerSessions,
  useRevokeCustomerSession,
} from "@/features/customer-panel/hooks/use-customer-panel";
import { customerErrorMessage } from "@/features/customer-panel/lib/errors";
import type { CustomerDevice } from "@/features/customer-panel/types";
import { datetime } from "@/lib/utils/format";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * The person's signed-in devices (live panel sessions). Any other device
 * can be signed out at once; this one signs out with the buttons below.
 */
export function CustomerDevicesCard() {
  const { t, locale } = useLocale();
  const sessions = useCustomerSessions();
  const revoke = useRevokeCustomerSession();
  const [target, setTarget] = useState<CustomerDevice | null>(null);
  const items = sessions.data?.items;

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerDevice>({
          accessorKey: "device",
          labelKey: "customer_panel.devices.device",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2 font-medium">
              {row.original.device || t("customer_panel.devices.unknown")}
              {row.original.current ? (
                <Badge variant="secondary">
                  {t("customer_panel.devices.current")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<CustomerDevice>({
          accessorKey: "method",
          labelKey: "customer_panel.devices.method",
          enableSorting: false,
          cell: ({ row }) =>
            t(`customer_panel.devices.methods.${row.original.method}`),
        }),
        createColumn<CustomerDevice>({
          accessorKey: "last_seen_at",
          labelKey: "customer_panel.devices.last_seen_at",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            datetime(row.original.last_seen_at, undefined, locale),
        }),
        createColumn<CustomerDevice>({
          accessorKey: "created_at",
          labelKey: "customer_panel.devices.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<CustomerDevice>({
          accessorKey: "ip",
          labelKey: "customer_panel.devices.ip",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.ip || "—"}</span>
          ),
        }),
        {
          id: "actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) =>
            row.original.current ? null : (
              <EntityRowActions
                actions={[
                  {
                    id: "revoke",
                    label: t("customer_panel.devices.revoke"),
                    icon: LogOut,
                    variant: "destructive",
                    onSelect: () => setTarget(row.original),
                  },
                ]}
              />
            ),
        },
      ] as ColumnDef<CustomerDevice, unknown>[],
    [locale, t],
  );

  const confirm = () => {
    if (!target) return;
    revoke.mutate(target.uuid, {
      onSuccess: () => appToast.success(t("customer_panel.devices.revoked")),
      onError: (error) => appToast.error(customerErrorMessage(t, error)),
      onSettled: () => setTarget(null),
    });
  };

  return (
    <EntitySectionCard
      title={t("customer_panel.devices.title")}
      badge={items?.length}
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t("customer_panel.devices.description")}
      </p>
      <EntityTable
        columns={columns}
        data={items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={sessions.isLoading}
        isError={sessions.isError}
        onRetry={() => void sessions.refetch()}
        emptyTitle={t("customer_panel.devices.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "customer-panel-devices-v1",
        }}
      />
      <ConfirmDialog
        open={target !== null}
        title={t("customer_panel.devices.revoke_title")}
        description={t("customer_panel.devices.revoke_description")}
        confirmLabel={t("customer_panel.devices.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onConfirm={confirm}
        onCancel={() => setTarget(null)}
      />
    </EntitySectionCard>
  );
}

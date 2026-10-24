"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { DeliveryDrawer } from "@/features/customer-notifications/components/delivery-drawer";
import {
  NOTIFICATION_CHANNELS,
  HISTORY_EVENTS,
  NOTIFICATION_STATUSES,
  STATUS_TONES,
  channelLabel,
  eventLabel,
  statusLabel,
} from "@/features/customer-notifications/constants";
import { useCustomerNotificationDeliveries } from "@/features/customer-notifications/hooks/use-customer-notifications";
import type {
  CustomerNotificationDelivery,
  ListCustomerNotificationDeliveriesParams,
} from "@/features/customer-notifications/types";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Sent notifications, one row per channel attempt, newest first. A channel
 * without a ready provider shows as "Atlandı", never as sent. Read-only.
 */
export function NotificationHistoryTab() {
  const { t, locale } = useLocale();
  const [selected, setSelected] = useState<CustomerNotificationDelivery | null>(
    null,
  );

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const listParams = useMemo<ListCustomerNotificationDeliveriesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const join = (values: string[]) =>
      values.length ? values.join(",") : undefined;
    return {
      ...listState.params,
      event: join(column("event")),
      channel: join(column("channel")),
      status: join(column("status")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useCustomerNotificationDeliveries(listParams);

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerNotificationDelivery>({
          accessorKey: "created_at",
          labelKey: "customer_notifications.columns.created_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <time
              dateTime={row.original.created_at}
              title={relativeDatetime(row.original.created_at, locale)}
              className="text-sm whitespace-nowrap tabular-nums"
            >
              {datetime(row.original.created_at, undefined, locale)}
            </time>
          ),
        }),
        createColumn<CustomerNotificationDelivery>({
          accessorKey: "event",
          labelKey: "customer_notifications.columns.event",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: HISTORY_EVENTS.map((code) => ({
            value: code,
            label: eventLabel(t, code),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {eventLabel(t, row.original.event)}
            </span>
          ),
        }),
        createColumn<CustomerNotificationDelivery>({
          id: "customer",
          accessorFn: (row) => row.customer_name,
          labelKey: "customer_notifications.columns.customer",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="truncate">{row.original.customer_name}</span>
          ),
        }),
        createColumn<CustomerNotificationDelivery>({
          id: "reservation",
          accessorFn: (row) => row.reservation_number ?? 0,
          labelKey: "customer_notifications.columns.reservation",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reservation_number ? (
              <span className="tabular-nums">
                #{row.original.reservation_number}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<CustomerNotificationDelivery>({
          accessorKey: "channel",
          labelKey: "customer_notifications.columns.channel",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: NOTIFICATION_CHANNELS.map((name) => ({
            value: name,
            label: channelLabel(t, name),
          })),
          cell: ({ row }) => channelLabel(t, row.original.channel),
        }),
        createColumn<CustomerNotificationDelivery>({
          id: "recipient",
          accessorFn: (row) => row.recipient_masked,
          labelKey: "customer_notifications.columns.recipient",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.recipient_masked ? (
              <span className="font-mono text-xs">
                {row.original.recipient_masked}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<CustomerNotificationDelivery>({
          accessorKey: "status",
          labelKey: "customer_notifications.columns.status",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: NOTIFICATION_STATUSES.map((status) => ({
            value: status,
            label: statusLabel(t, status),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={statusLabel(t, row.original.status)}
              tone={STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<CustomerNotificationDelivery>({
          accessorKey: "error",
          labelKey: "customer_notifications.columns.error",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground line-clamp-2 text-xs">
              {row.original.error || "—"}
            </span>
          ),
        }),
        createColumn<CustomerNotificationDelivery>({
          accessorKey: "provider",
          labelKey: "customer_notifications.columns.provider",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.provider || "—",
        }),
      ] as ColumnDef<CustomerNotificationDelivery, unknown>[],
    [locale, t],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.event) ||
    Boolean(listParams.channel) ||
    Boolean(listParams.status);

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={setSelected}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("customer_notifications.history.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters
            ? t("customer_notifications.history.empty_filtered_title")
            : t("customer_notifications.history.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("customer_notifications.history.empty_filtered_description")
            : t("customer_notifications.history.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-customer-notification-history-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <DeliveryDrawer
        delivery={selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      />
    </>
  );
}

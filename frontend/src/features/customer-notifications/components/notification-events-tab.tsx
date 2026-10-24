"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { NotificationEventDialog } from "@/features/customer-notifications/components/notification-event-dialog";
import {
  EVENT_HINT_KEYS,
  channelLabel,
  eventLabel,
  offsetLabel,
} from "@/features/customer-notifications/constants";
import {
  useCustomerNotificationSettings,
  useResetCustomerNotification,
} from "@/features/customer-notifications/hooks/use-customer-notifications";
import type { CustomerNotificationEvent } from "@/features/customer-notifications/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const STATIC_TABLE = {
  manual: { filtering: false, sorting: false, pagination: false },
  features: {
    rowSelection: false,
    columnFilters: false,
    facetedFilters: false,
    sorting: false,
    pagination: false,
  },
} as const;

/**
 * The customer notification events with their rule: on / off, channels in
 * sending order and reminder times. A row opens the editor; "Varsayılana
 * dön" drops the business's rule and texts.
 */
export function NotificationEventsTab() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const settings = useCustomerNotificationSettings();
  const reset = useResetCustomerNotification();
  const [editing, setEditing] = useState<string | null>(null);
  const [resetting, setResetting] = useState<CustomerNotificationEvent | null>(
    null,
  );

  const events = useMemo(
    () => settings.data?.events ?? [],
    [settings.data?.events],
  );
  const channels = useMemo(
    () => settings.data?.channels ?? [],
    [settings.data?.channels],
  );
  const readiness = useMemo(
    () =>
      Object.fromEntries(channels.map((c) => [c.name, c.ready])) as Record<
        string,
        boolean
      >,
    [channels],
  );
  const notReady = channels.filter((c) => !c.ready);
  const editingEvent = events.find((e) => e.code === editing) ?? null;

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerNotificationEvent>({
          accessorKey: "code",
          labelKey: "customer_notifications.columns.event",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="truncate font-medium">
                {eventLabel(t, row.original.code)}
              </p>
              <p className="text-muted-foreground line-clamp-2 text-xs">
                {t(EVENT_HINT_KEYS[row.original.code])}
              </p>
            </div>
          ),
        }),
        createColumn<CustomerNotificationEvent>({
          id: "enabled",
          accessorFn: (row) => row.rule.enabled,
          labelKey: "customer_notifications.columns.state",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1.5">
              <StatusChip
                label={t(
                  row.original.rule.enabled
                    ? "customer_notifications.state.on"
                    : "customer_notifications.state.off",
                )}
                tone={row.original.rule.enabled ? "success" : "default"}
              />
              {row.original.custom ? (
                <Badge variant="outline">
                  {t("customer_notifications.state.custom")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<CustomerNotificationEvent>({
          id: "channels",
          accessorFn: (row) => row.rule.channels.join(","),
          labelKey: "customer_notifications.columns.channel",
          enableSorting: false,
          cell: ({ row }) => {
            const list = row.original.rule.channels;
            if (!list.length) {
              return <span className="text-muted-foreground">—</span>;
            }
            return (
              <div className="min-w-0">
                <p className="truncate">
                  {list.map((c) => channelLabel(t, c)).join(" → ")}
                </p>
                {list.length > 1 ? (
                  <p className="text-muted-foreground text-xs">
                    {t(
                      row.original.rule.fallback
                        ? "customer_notifications.state.fallback_on"
                        : "customer_notifications.state.fallback_off",
                    )}
                  </p>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<CustomerNotificationEvent>({
          id: "reminder_offsets",
          accessorFn: (row) => row.rule.reminder_offsets.join(","),
          labelKey: "customer_notifications.columns.timing",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reminder ? (
              <span>
                {row.original.rule.reminder_offsets
                  .map((m) => offsetLabel(t, m))
                  .join(", ") || "—"}
              </span>
            ) : (
              <span className="text-muted-foreground">
                {t("customer_notifications.state.on_change")}
              </span>
            ),
        }),
        createColumn<CustomerNotificationEvent>({
          id: "actions",
          labelKey: "customer_notifications.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t(
                    canWrite
                      ? "customer_notifications.actions.edit"
                      : "customer_notifications.actions.view",
                  ),
                  icon: Pencil,
                  onSelect: () => setEditing(row.original.code),
                },
                {
                  id: "reset",
                  label: t("customer_notifications.actions.reset"),
                  icon: RotateCcw,
                  permission: permissions.tenant.settings.write,
                  disabled: !row.original.custom,
                  onSelect: () => setResetting(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CustomerNotificationEvent, unknown>[],
    [canWrite, t],
  );

  return (
    <div className="space-y-4">
      {notReady.length ? (
        <Alert>
          {/* The channel list must stay readable at phone width. */}
          <AlertTitle className="line-clamp-none">
            {t("customer_notifications.readiness.title", {
              channels: notReady.map((c) => channelLabel(t, c.name)).join(", "),
            })}
          </AlertTitle>
          <AlertDescription>
            {t("customer_notifications.readiness.description")}
          </AlertDescription>
        </Alert>
      ) : null}

      <EntityTable
        columns={columns}
        data={events}
        getRowId={(row) => row.code}
        onRowClick={(row) => setEditing(row.code)}
        isLoading={settings.isLoading}
        isError={settings.isError}
        errorDescription={t("customer_notifications.load_failed")}
        onRetry={() => void settings.refetch()}
        emptyTitle={t("customer_notifications.empty_events")}
        {...STATIC_TABLE}
        features={{
          ...STATIC_TABLE.features,
          persistKey: "tenant-customer-notification-events-v1",
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void settings.refetch()}
            refreshDisabled={settings.isFetching}
          />
        }
      />

      <NotificationEventDialog
        event={editingEvent}
        readiness={readiness}
        readOnly={!canWrite}
        onClose={() => setEditing(null)}
      />

      <ConfirmDialog
        open={Boolean(resetting)}
        title={t("customer_notifications.reset_title")}
        description={
          resetting
            ? t("customer_notifications.reset_description", {
                event: eventLabel(t, resetting.code),
              })
            : undefined
        }
        confirmLabel={t("customer_notifications.actions.reset")}
        isPending={reset.isPending}
        onConfirm={() => {
          if (!resetting) return;
          reset.mutate(resetting.code, {
            onSuccess: () => setResetting(null),
          });
        }}
        onCancel={() => setResetting(null)}
      />
    </div>
  );
}

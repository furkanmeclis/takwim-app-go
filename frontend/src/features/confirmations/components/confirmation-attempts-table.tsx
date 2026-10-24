"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityTable, EntityToolbar } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { useConfirmationAttempts } from "@/features/confirmations/hooks/use-confirmations";
import { SEND_STATUS } from "@/features/confirmations/lib/list";
import type { ConfirmationAttempt } from "@/features/confirmations/types";
import { channelLabel } from "@/features/customer-notifications/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

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
 * The confirmation requests of one booking: every channel tried, the
 * provider's result, the masked recipient and the times (requirements §9:
 * acceptance, delivery and the answer are kept apart).
 */
export function ConfirmationAttemptsTable({
  reservationUuid,
}: {
  reservationUuid: string;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  const query = useConfirmationAttempts(reservationUuid);

  const columns = useMemo(() => {
    const when = (iso: string | null | undefined) =>
      iso
        ? datetimeInZone(iso, zone, locale, {
            dateStyle: "short",
            timeStyle: "short",
          })
        : "—";
    return [
      createColumn<ConfirmationAttempt>({
        id: "request",
        accessorFn: (row) => `${row.round}-${row.request}`,
        labelKey: "confirmations.attempts.columns.request",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) =>
          t("confirmations.attempts.request_value", {
            request: row.original.request,
            round: row.original.round,
          }),
      }),
      createColumn<ConfirmationAttempt>({
        id: "channel",
        accessorFn: (row) => row.channel ?? "",
        labelKey: "confirmations.attempts.columns.channel",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) =>
          row.original.channel
            ? channelLabel(t, row.original.channel)
            : t("confirmations.attempts.no_channel"),
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "status",
        labelKey: "confirmations.attempts.columns.status",
        enableSorting: false,
        cell: ({ row }) => {
          const status = SEND_STATUS[row.original.status];
          return status ? (
            <StatusChip label={t(status.key)} tone={status.tone} />
          ) : (
            row.original.status
          );
        },
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "recipient",
        labelKey: "confirmations.attempts.columns.recipient",
        enableSorting: false,
        cell: ({ row }) => row.original.recipient || "—",
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "scheduled_at",
        labelKey: "confirmations.attempts.columns.scheduled_at",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {when(row.original.scheduled_at)}
          </span>
        ),
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "sent_at",
        labelKey: "confirmations.attempts.columns.sent_at",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {when(row.original.finished_at ?? row.original.sent_at)}
          </span>
        ),
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "error",
        labelKey: "confirmations.attempts.columns.error",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.error ? (
            <span className="text-destructive line-clamp-2 text-xs">
              {row.original.error}
            </span>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "provider",
        labelKey: "confirmations.attempts.columns.provider",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.provider || "—",
      }),
    ] as ColumnDef<ConfirmationAttempt, unknown>[];
  }, [locale, t, zone]);

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) =>
        `${row.dispatch_uuid}-${row.delivery_uuid ?? "pending"}`
      }
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("confirmations.attempts.load_failed")}
      onRetry={() => void query.refetch()}
      emptyTitle={t("confirmations.attempts.empty_title")}
      emptyDescription={t("confirmations.attempts.empty_description")}
      {...STATIC_TABLE}
      features={{
        ...STATIC_TABLE.features,
        persistKey: "tenant-confirmation-attempts-v1",
        viewMode: true,
      }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void query.refetch()}
          refreshDisabled={query.isFetching}
        />
      }
    />
  );
}

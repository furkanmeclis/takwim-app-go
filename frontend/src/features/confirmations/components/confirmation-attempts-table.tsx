"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityTable, EntityToolbar } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { useConfirmationAttempts } from "@/features/confirmations/hooks/use-confirmations";
import {
  RESPONSE_REASONS,
  RESPONSE_RESULT,
  SEND_STATUS,
} from "@/features/confirmations/lib/list";
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
 * acceptance, delivery, read and the customer's reply are kept apart; a
 * delivered or read message is never a confirmation, TEC-922).
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
    const reasonLabel = (reason: string) =>
      (RESPONSE_REASONS as readonly string[]).includes(reason)
        ? t(`confirmations.responses.reasons.${reason}`)
        : reason;
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
            {when(row.original.sent_at ?? row.original.finished_at)}
          </span>
        ),
      }),
      createColumn<ConfirmationAttempt>({
        accessorKey: "delivered_at",
        labelKey: "confirmations.attempts.columns.delivered_at",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.delivered_at ? (
            <span className="flex flex-col tabular-nums">
              <span>{when(row.original.delivered_at)}</span>
              {row.original.read_at ? (
                <span className="text-muted-foreground text-xs">
                  {t("confirmations.attempts.read_value", {
                    time: when(row.original.read_at),
                  })}
                </span>
              ) : null}
            </span>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<ConfirmationAttempt>({
        id: "response",
        accessorFn: (row) => row.responses.length,
        labelKey: "confirmations.attempts.columns.response",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.responses.length ? (
            <ul className="space-y-1">
              {row.original.responses.map((r) => {
                const result = RESPONSE_RESULT[r.result];
                return (
                  <li key={r.uuid} className="flex flex-col gap-0.5">
                    <span className="flex flex-wrap items-center gap-1.5">
                      <span className="text-sm">
                        {t(`confirmations.responses.answers.${r.answer}`)}
                      </span>
                      {result ? (
                        <StatusChip label={t(result.key)} tone={result.tone} />
                      ) : null}
                    </span>
                    <span className="text-muted-foreground text-xs tabular-nums">
                      {when(r.responded_at)}
                      {r.reason ? ` · ${reasonLabel(r.reason)}` : ""}
                    </span>
                  </li>
                );
              })}
            </ul>
          ) : (
            <span className="text-muted-foreground">
              {t("confirmations.attempts.no_response")}
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

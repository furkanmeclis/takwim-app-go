"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import {
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  channelLabel,
  errorLabel,
  eventLabel,
} from "@/features/customer-notifications/constants";
import { useNoticeFailures } from "@/features/operations/hooks/use-operations";
import type { NoticeFailure } from "@/features/operations/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DATE = "d MMM yyyy HH:mm:ss";
const CHANNELS = ["sms", "whatsapp", "email", "voice_call"];

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  return [];
}

/**
 * Failed customer notice sends of every business (TEC-930). Read only: a
 * channel is tried at most once per notice, so nothing is sent again from
 * here. No recipient and no text are shown.
 */
export function NoticeFailuresPanel() {
  const { t, locale } = useLocale();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const channel = filterValues(
    listState.columnFilters.find((f) => f.id === "channel")?.value,
  );
  const params = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    sort: listState.params.sort,
    q: listState.params.q,
    channel: channel.length ? channel.join(",") : undefined,
  };
  const query = useNoticeFailures(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<NoticeFailure>({
          id: "organization",
          labelKey: "operations.integrations.columns.organization",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.organization.name}
            </span>
          ),
        }),
        createColumn<NoticeFailure>({
          accessorKey: "event_code",
          labelKey: "operations.notices.columns.event",
          enableSorting: false,
          cell: ({ row }) => eventLabel(t, row.original.event_code),
        }),
        createColumn<NoticeFailure>({
          accessorKey: "channel",
          labelKey: "operations.notices.columns.channel",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CHANNELS.map((c) => ({
            value: c,
            label: channelLabel(t, c),
          })),
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>{channelLabel(t, row.original.channel)}</span>
              {row.original.provider ? (
                <span className="text-muted-foreground text-xs">
                  {row.original.provider}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<NoticeFailure>({
          accessorKey: "error",
          labelKey: "operations.notices.columns.error",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-destructive line-clamp-2 text-xs break-words">
              {errorLabel(t, row.original.error) || "—"}
            </span>
          ),
        }),
        createColumn<NoticeFailure>({
          accessorKey: "created_at",
          labelKey: "operations.notices.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.created_at, DATE, locale),
        }),
      ] as ColumnDef<NoticeFailure, unknown>[],
    [locale, t],
  );

  return (
    <EntitySectionCard title={t("operations.notices.title")}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("operations.notices.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("operations.notices.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("operations.notices.empty_title")}
        emptyDescription={t("operations.notices.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "platform-operations-notice-failures-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </EntitySectionCard>
  );
}

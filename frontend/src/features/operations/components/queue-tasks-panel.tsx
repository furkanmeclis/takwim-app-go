"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Play, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import {
  useDeleteQueueTask,
  useOperationsSummary,
  useQueueTasks,
  useRunQueueTask,
} from "@/features/operations/hooks/use-operations";
import type { QueueTask, QueueTaskState } from "@/features/operations/types";
import { datetime } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const DATE = "d MMM yyyy HH:mm:ss";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  return [];
}

/**
 * Failed background tasks (TEC-930): archived = out of automatic attempts,
 * retry = waiting for the next automatic attempt. A task can be run again
 * now or dropped; its own job decides again whether work is left.
 */
export function QueueTasksPanel() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const { confirm, confirmDelete } = useDialogs();
  const canWrite = can(permissions.settings.write);
  const [state, setState] = useState<QueueTaskState>("archived");
  const listState = useServerListState({ initialPageSize: 20 });
  const summary = useOperationsSummary();
  const queueNames = useMemo(
    () => (summary.data?.queue.queues ?? []).map((q) => q.name),
    [summary.data],
  );
  const queueFilter = filterValues(
    listState.columnFilters.find((f) => f.id === "queue")?.value,
  );
  const params = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    state,
    queue: queueFilter.length ? queueFilter.join(",") : undefined,
  };
  const query = useQueueTasks(params);
  const run = useRunQueueTask();
  const remove = useDeleteQueueTask();
  const total = query.data?.total ?? 0;

  const onRun = useCallback(
    async (task: QueueTask) => {
      const ok = await confirm({
        title: t("operations.queue.run_title"),
        description: t("operations.queue.run_description", { type: task.type }),
        confirmLabel: t("operations.queue.actions.run"),
      });
      if (ok) run.mutate(task);
    },
    [confirm, run, t],
  );
  const onDelete = useCallback(
    async (task: QueueTask) => {
      const ok = await confirmDelete({
        title: t("operations.queue.delete_title"),
        description: t("operations.queue.delete_description", {
          type: task.type,
        }),
      });
      if (ok) remove.mutate(task);
    },
    [confirmDelete, remove, t],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<QueueTask>({
          accessorKey: "type",
          labelKey: "operations.queue.columns.type",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span className="font-mono text-xs font-medium">
                {row.original.type}
              </span>
              <span className="text-muted-foreground font-mono text-xs">
                {row.original.id.slice(0, 12)}
              </span>
            </div>
          ),
        }),
        createColumn<QueueTask>({
          accessorKey: "queue",
          labelKey: "operations.queue.columns.queue",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: queueNames.map((name) => ({
            value: name,
            label: name,
          })),
          cell: ({ row }) => (
            <span className="font-mono text-xs">{row.original.queue}</span>
          ),
        }),
        createColumn<QueueTask>({
          accessorKey: "retried",
          labelKey: "operations.queue.columns.retried",
          enableSorting: false,
          cell: ({ row }) =>
            `${row.original.retried} / ${row.original.max_retry}`,
        }),
        createColumn<QueueTask>({
          accessorKey: "last_error",
          labelKey: "operations.queue.columns.last_error",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.last_error ? (
              <span className="text-destructive line-clamp-2 text-xs break-words">
                {row.original.last_error}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<QueueTask>({
          accessorKey: "last_failed_at",
          labelKey: "operations.queue.columns.last_failed_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.last_failed_at
              ? datetime(row.original.last_failed_at, DATE, locale)
              : "—",
        }),
        createColumn<QueueTask>({
          accessorKey: "next_process_at",
          labelKey: "operations.queue.columns.next_process_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.state === "retry" && row.original.next_process_at
              ? datetime(row.original.next_process_at, DATE, locale)
              : "—",
        }),
        createColumn<QueueTask>({
          accessorKey: "state",
          labelKey: "operations.queue.columns.state",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                row.original.state === "archived"
                  ? "operations.queue.states.archived"
                  : "operations.queue.states.retry",
              )}
              tone={row.original.state === "archived" ? "danger" : "warning"}
            />
          ),
        }),
        createColumn<QueueTask>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canWrite ? (
              <EntityRowActions
                actions={[
                  {
                    id: "run",
                    label: t("operations.queue.actions.run"),
                    icon: Play,
                    onSelect: () => void onRun(row.original),
                  },
                  {
                    id: "delete",
                    label: t("common.delete"),
                    icon: Trash2,
                    variant: "destructive" as const,
                    onSelect: () => void onDelete(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<QueueTask, unknown>[],
    [canWrite, locale, onDelete, onRun, queueNames, t],
  );

  return (
    <EntitySectionCard
      title={t("operations.queue.title")}
      actions={
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={state}
          onValueChange={(next) => {
            if (next !== "archived" && next !== "retry") return;
            setState(next);
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
        >
          <ToggleGroupItem value="archived">
            {t("operations.queue.states.archived")}
          </ToggleGroupItem>
          <ToggleGroupItem value="retry">
            {t("operations.queue.states.retry")}
          </ToggleGroupItem>
        </ToggleGroup>
      }
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t(
          state === "archived"
            ? "operations.queue.archived_hint"
            : "operations.queue.retry_hint",
        )}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => `${row.queue}/${row.id}`}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("operations.queue.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("operations.queue.empty_title")}
        emptyDescription={t("operations.queue.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "platform-operations-queue-tasks-v1",
          rowSelection: false,
          globalFilter: false,
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

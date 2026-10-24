"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Copy, Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { ClassDialog } from "@/features/classes/components/class-dialog";
import {
  CLASSES_RESOURCE,
  WEEKDAY_LABEL_KEYS,
  WEEKDAYS,
} from "@/features/classes/constants";
import {
  useClassFacets,
  useClasses,
  useDeleteClass,
} from "@/features/classes/hooks/use-classes";
import {
  facetOptions,
  filterValues,
  joined,
} from "@/features/classes/lib/filters";
import type {
  ClassFacets,
  ClassSchedule,
  ListClassesParams,
} from "@/features/classes/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function useTimetableColumns({
  facets,
  onEdit,
  onCopy,
  onDelete,
}: {
  facets?: ClassFacets;
  onEdit?: (row: ClassSchedule) => void;
  onCopy?: (row: ClassSchedule) => void;
  onDelete?: (row: ClassSchedule) => void;
}) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const columns = [
      createColumn<ClassSchedule>({
        id: "service",
        accessorFn: (row) => row.service.name,
        labelKey: "classes.columns.service",
        gridPrimary: true,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.services),
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium">{row.original.service.name}</p>
            {row.original.note ? (
              <p className="text-muted-foreground line-clamp-1 text-xs">
                {row.original.note}
              </p>
            ) : null}
          </div>
        ),
      }),
      createColumn<ClassSchedule>({
        accessorKey: "weekday",
        labelKey: "classes.columns.weekday",
        gridSecondary: true,
        filterVariant: "faceted",
        filterOptions: WEEKDAYS.map((day) => ({
          value: day,
          label: t(WEEKDAY_LABEL_KEYS[day]),
        })),
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="font-medium">
              {t(WEEKDAY_LABEL_KEYS[row.original.weekday])}
            </span>
            <span className="text-muted-foreground text-xs tabular-nums">
              {row.original.start_time}–{row.original.end_time}
            </span>
          </div>
        ),
      }),
      createColumn<ClassSchedule>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "classes.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.branches),
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<ClassSchedule>({
        id: "staff",
        accessorFn: (row) => row.staff?.name ?? "",
        labelKey: "classes.columns.staff",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.staff),
        cell: ({ row }) =>
          row.original.staff?.name ?? (
            <span className="text-muted-foreground text-sm">
              {t("classes.picked_on_booking")}
            </span>
          ),
      }),
      createColumn<ClassSchedule>({
        id: "resource",
        accessorFn: (row) => row.resource?.name ?? "",
        labelKey: "classes.columns.resource",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.resource?.name ?? (
            <span className="text-muted-foreground text-sm">—</span>
          ),
      }),
      createColumn<ClassSchedule>({
        accessorKey: "seats",
        labelKey: "classes.columns.seats",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {row.original.seats}
            {row.original.capacity == null ? (
              <span className="text-muted-foreground ml-1 text-xs">
                ({t("classes.seats_from_service")})
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<ClassSchedule>({
        id: "validity",
        accessorFn: (row) => row.valid_from,
        labelKey: "classes.columns.validity",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.valid_until
            ? t("classes.validity_range", {
                from: date(row.original.valid_from, undefined, locale),
                until: date(row.original.valid_until, undefined, locale),
              })
            : t("classes.validity_open", {
                from: date(row.original.valid_from, undefined, locale),
              }),
      }),
      createColumn<ClassSchedule>({
        accessorKey: "is_active",
        labelKey: "classes.columns.is_active",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: [
          { value: "true", label: t("classes.active.yes") },
          { value: "false", label: t("classes.active.no") },
        ],
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? t("classes.active.yes")
                : t("classes.active.no")
            }
            tone={row.original.is_active ? "success" : "default"}
          />
        ),
      }),
      createColumn<ClassSchedule>({
        accessorKey: "created_at",
        labelKey: "classes.columns.created_at",
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      ...(onEdit && onCopy && onDelete
        ? [
            createColumn<ClassSchedule>({
              id: "actions",
              labelKey: "classes.columns.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) => (
                <EntityRowActions
                  actions={[
                    {
                      id: "edit",
                      label: t("classes.actions.edit"),
                      icon: Pencil,
                      permission: permissions.tenant.classes.write,
                      onSelect: () => onEdit(row.original),
                    },
                    {
                      id: "copy",
                      label: t("classes.actions.copy"),
                      icon: Copy,
                      permission: permissions.tenant.classes.write,
                      onSelect: () => onCopy(row.original),
                    },
                    {
                      id: "delete",
                      label: t("classes.actions.delete"),
                      icon: Trash2,
                      permission: permissions.tenant.classes.write,
                      variant: "destructive",
                      onSelect: () => onDelete(row.original),
                    },
                  ]}
                />
              ),
            }),
          ]
        : []),
    ];
    return columns as ColumnDef<ClassSchedule, unknown>[];
  }, [facets, locale, onCopy, onDelete, onEdit, t]);
}

/**
 * The weekly classes (server list: search, sort, branch / class /
 * trainer / day / active filters). Managing them needs
 * `tenant.classes.write`.
 */
export function TimetableTable({
  canWrite,
  creating,
  onCreatingChange,
}: {
  canWrite: boolean;
  creating: boolean;
  onCreatingChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "weekday",
    initialPageSize: 50,
    realtime: tenantLiveList(organization, CLASSES_RESOURCE),
  });
  const facets = useClassFacets();
  const [editing, setEditing] = useState<ClassSchedule | null>(null);
  const [copying, setCopying] = useState<ClassSchedule | null>(null);
  const [deleting, setDeleting] = useState<ClassSchedule | null>(null);
  const remove = useDeleteClass();

  const params = useMemo<ListClassesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const active = column("is_active");
    return {
      ...listState.params,
      service: joined(column("service")),
      branch: joined(column("branch")),
      staff: joined(column("staff")),
      weekday: joined(column("weekday")),
      is_active:
        active.length === 1 ? (active[0] as "true" | "false") : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const query = useClasses(params);
  const columns = useTimetableColumns({
    facets: facets.data,
    ...(canWrite
      ? { onEdit: setEditing, onCopy: setCopying, onDelete: setDeleting }
      : {}),
  });
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 50)),
  );
  const hasFilters = Boolean(
    params.q ||
    params.service ||
    params.branch ||
    params.staff ||
    params.weekday ||
    params.is_active,
  );
  const dialogRow = editing ?? copying;

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={canWrite ? setEditing : undefined}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("classes.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([query.refetch(), facets.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("classes.empty_filtered_title")
            : t("classes.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("classes.empty_filtered_description")
            : t("classes.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-class-timetable-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void query.refetch();
              void facets.refetch();
            }}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <ClassDialog
        schedule={dialogRow}
        copy={Boolean(copying)}
        open={creating || Boolean(dialogRow)}
        onOpenChange={(open) => {
          if (open) return;
          setEditing(null);
          setCopying(null);
          onCreatingChange(false);
        }}
      />
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={
          deleting
            ? `${deleting.service.name} · ${t(WEEKDAY_LABEL_KEYS[deleting.weekday])} ${deleting.start_time}`
            : ""
        }
        description={t("classes.delete_description")}
        isPending={remove.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() => {
          if (!deleting) return;
          remove.mutate(deleting.uuid, { onSettled: () => setDeleting(null) });
        }}
      />
    </>
  );
}

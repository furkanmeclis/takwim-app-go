"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Eye, RotateCcw, Users } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  SessionChangeDialog,
  type SessionChangeMode,
} from "@/features/classes/components/session-change-dialog";
import {
  CLASSES_RESOURCE,
  SESSION_STATE_LABEL_KEYS,
  SESSION_STATE_TONES,
  SESSION_STATES,
  WEEKDAY_LABEL_KEYS,
} from "@/features/classes/constants";
import {
  useChangeClassSession,
  useClassFacets,
  useClassSessions,
} from "@/features/classes/hooks/use-classes";
import {
  facetOptions,
  filterValues,
  joined,
} from "@/features/classes/lib/filters";
import type {
  ClassFacets,
  ClassSession,
  ListClassSessionsParams,
} from "@/features/classes/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** "Pzt 14.06 · 18:00–19:00" in the branch's time zone. */
export function useSessionWhen() {
  const { t, locale } = useLocale();
  return useCallback(
    (s: ClassSession) => {
      const day = datetimeInZone(s.starts_at, s.timezone, locale, {
        day: "2-digit",
        month: "2-digit",
        year: "numeric",
      });
      const clock = (value: string) =>
        datetimeInZone(value, s.timezone, locale, {
          hour: "2-digit",
          minute: "2-digit",
        });
      return {
        day: `${t(WEEKDAY_LABEL_KEYS[s.weekday])} ${day}`,
        time: `${clock(s.starts_at)}–${clock(s.ends_at)}`,
      };
    },
    [locale, t],
  );
}

function useSessionColumns({
  facets,
  onCancel,
  onSeats,
  onRestore,
  onOpen,
}: {
  facets?: ClassFacets;
  onCancel?: (row: ClassSession) => void;
  onSeats?: (row: ClassSession) => void;
  onRestore?: (row: ClassSession) => void;
  onOpen: (row: ClassSession) => void;
}) {
  const { t } = useLocale();
  const when = useSessionWhen();

  return useMemo(() => {
    const columns = [
      createColumn<ClassSession>({
        id: "starts_at",
        accessorFn: (row) => row.starts_at,
        labelKey: "classes.sessions.columns.starts_at",
        cell: ({ row }) => {
          const w = when(row.original);
          return (
            <div className="flex flex-col">
              <span className="font-medium">{w.day}</span>
              <span className="text-muted-foreground text-xs tabular-nums">
                {w.time}
              </span>
            </div>
          );
        },
      }),
      createColumn<ClassSession>({
        id: "service",
        accessorFn: (row) => row.service.name,
        labelKey: "classes.sessions.columns.service",
        gridPrimary: true,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.services),
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium">{row.original.service.name}</p>
            <p className="text-muted-foreground truncate text-xs">
              {[row.original.staff?.name, row.original.resource?.name]
                .filter(Boolean)
                .join(" · ") || t("classes.picked_on_booking")}
            </p>
          </div>
        ),
      }),
      createColumn<ClassSession>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "classes.sessions.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.branches),
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<ClassSession>({
        id: "staff",
        accessorFn: (row) => row.staff?.name ?? "",
        labelKey: "classes.sessions.columns.staff",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.staff),
        cell: ({ row }) => row.original.staff?.name ?? "—",
      }),
      createColumn<ClassSession>({
        id: "booked",
        accessorFn: (row) => row.booked,
        labelKey: "classes.sessions.columns.booked",
        gridSecondary: true,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="font-medium tabular-nums">
              {t("classes.sessions.booked_value", {
                booked: row.original.booked,
                capacity: row.original.capacity,
              })}
            </span>
            {row.original.capacity_changed ? (
              <span className="text-muted-foreground text-xs">
                {t("classes.sessions.capacity_changed")}
              </span>
            ) : null}
          </div>
        ),
      }),
      createColumn<ClassSession>({
        accessorKey: "state",
        labelKey: "classes.sessions.columns.state",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: SESSION_STATES.map((state) => ({
          value: state,
          label: t(SESSION_STATE_LABEL_KEYS[state]),
        })),
        cell: ({ row }) => (
          <div className="flex flex-col items-start gap-1">
            <StatusChip
              label={t(SESSION_STATE_LABEL_KEYS[row.original.state])}
              tone={SESSION_STATE_TONES[row.original.state]}
            />
            {row.original.reason ? (
              <span className="text-muted-foreground line-clamp-1 text-xs">
                {row.original.reason}
              </span>
            ) : null}
          </div>
        ),
      }),
      createColumn<ClassSession>({
        id: "actions",
        labelKey: "classes.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const s = row.original;
          const open = s.state !== "past";
          return (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("classes.actions.open_session"),
                  icon: Eye,
                  onSelect: () => onOpen(s),
                },
                ...(onSeats && open
                  ? [
                      {
                        id: "seats",
                        label: t("classes.actions.change_seats"),
                        icon: Users,
                        permission: permissions.tenant.classes.write,
                        onSelect: () => onSeats(s),
                      },
                    ]
                  : []),
                ...(onRestore && open && s.cancelled
                  ? [
                      {
                        id: "restore",
                        label: t("classes.actions.restore_session"),
                        icon: RotateCcw,
                        permission: permissions.tenant.classes.write,
                        onSelect: () => onRestore(s),
                      },
                    ]
                  : []),
                ...(onCancel && open && !s.cancelled
                  ? [
                      {
                        id: "cancel",
                        // A booked session is cancelled with its bookings,
                        // which also needs the reservations permission.
                        label:
                          s.booked > 0
                            ? t("classes.actions.cancel_with_bookings")
                            : t("classes.actions.cancel_session"),
                        icon: Ban,
                        permission:
                          s.booked > 0
                            ? permissions.tenant.reservations.write
                            : permissions.tenant.classes.write,
                        variant: "destructive" as const,
                        onSelect: () => onCancel(s),
                      },
                    ]
                  : []),
              ]}
            />
          );
        },
      }),
    ];
    return columns as ColumnDef<ClassSession, unknown>[];
  }, [facets, onCancel, onOpen, onRestore, onSeats, t, when]);
}

/**
 * Sessions of the timetable on the chosen days (server list: search,
 * sort, branch / class / trainer / state filters). A row opens the
 * session with its bookings and attendance.
 */
export function SessionsTable({
  canWrite,
  range,
}: {
  canWrite: boolean;
  range: { from: string; to: string };
}) {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "starts_at",
    initialPageSize: 50,
    realtime: tenantLiveList(organization, CLASSES_RESOURCE),
  });
  const facets = useClassFacets();
  const [changing, setChanging] = useState<{
    session: ClassSession;
    mode: SessionChangeMode;
  } | null>(null);
  const [restoring, setRestoring] = useState<ClassSession | null>(null);
  const change = useChangeClassSession();

  const params = useMemo<ListClassSessionsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      from: range.from,
      to: range.to,
      service: joined(column("service")),
      branch: joined(column("branch")),
      staff: joined(column("staff")),
      state: joined(column("state")),
    };
  }, [listState.columnFilters, listState.params, range.from, range.to]);

  const query = useClassSessions(params);
  const openSession = useCallback(
    (row: ClassSession) =>
      router.push(
        routes.tenant.classes.session(slug, row.schedule_uuid, row.date),
      ),
    [router, slug],
  );
  const columns = useSessionColumns({
    facets: facets.data,
    onOpen: openSession,
    ...(canWrite
      ? {
          onCancel: (session) => setChanging({ session, mode: "cancel" }),
          onSeats: (session) => setChanging({ session, mode: "seats" }),
          onRestore: setRestoring,
        }
      : {}),
  });
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 50)),
  );
  const hasFilters = Boolean(
    params.q || params.service || params.branch || params.staff || params.state,
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => `${row.schedule_uuid}:${row.date}`}
        onRowClick={openSession}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("classes.sessions.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("classes.empty_filtered_title")
            : t("classes.sessions.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("classes.empty_filtered_description")
            : t("classes.sessions.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-class-sessions-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      {changing ? (
        <SessionChangeDialog
          session={changing.session}
          mode={changing.mode}
          open
          onOpenChange={(open) => {
            if (!open) setChanging(null);
          }}
        />
      ) : null}
      <ConfirmDialog
        open={Boolean(restoring)}
        title={t("classes.session_dialog.restore_title")}
        description={t("classes.session_dialog.restore_description")}
        confirmLabel={t("classes.actions.restore_session")}
        isPending={change.isPending}
        onCancel={() => setRestoring(null)}
        onConfirm={() => {
          if (!restoring) return;
          change.mutate(
            {
              uuid: restoring.schedule_uuid,
              date: restoring.date,
              body: {
                cancelled: false,
                capacity: restoring.capacity_changed
                  ? restoring.capacity
                  : null,
                reason: "",
              },
            },
            { onSettled: () => setRestoring(null) },
          );
        }}
      />
    </>
  );
}

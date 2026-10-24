"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  CalendarClock,
  Eye,
  ListChecks,
  Phone,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ConfirmByPhoneDialog,
  type PhoneConfirmTarget,
} from "@/features/confirmations/components/confirm-by-phone-dialog";
import { ConfirmationAlerts } from "@/features/confirmations/components/confirmation-alerts";
import { ConfirmationDrawer } from "@/features/confirmations/components/confirmation-drawer";
import {
  useConfirmationFacets,
  useConfirmationList,
  useConfirmationSummary,
} from "@/features/confirmations/hooks/use-confirmations";
import {
  ANSWER_KEYS,
  ANSWER_SOURCES,
  PERIODS,
  SEND_STATUS,
  STATES,
  isActiveVisit,
  periodRange,
  telHref,
  type Period,
} from "@/features/confirmations/lib/list";
import type {
  ConfirmationState,
  ConfirmationTask,
  ListConfirmationsParams,
} from "@/features/confirmations/types";
import { channelLabel } from "@/features/customer-notifications/constants";
import {
  ReservationActionDialog,
  type ActionTarget,
} from "@/features/reservations/components/reservation-action-dialog";
import {
  ApprovalChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import { RescheduleDialog } from "@/features/reservations/components/reschedule-dialog";
import { RESERVATIONS_RESOURCE } from "@/features/reservations/constants";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Views of the toolbar: the follow-ups, then periods of the visit start. */
type View = "attention" | Period;
const VIEWS: View[] = ["attention", ...PERIODS];
const DEFAULT_VIEW: View = "today";
const VIEW_KEYS: Record<Period, string> = {
  today: "confirmations.list.view.today",
  tomorrow: "confirmations.list.view.tomorrow",
  upcoming: "confirmations.list.view.upcoming",
  all: "confirmations.list.view.all",
};
const EMPTY_KEYS: Record<View, string> = {
  attention: "confirmations.list.empty.attention",
  today: "confirmations.list.empty.today",
  tomorrow: "confirmations.list.empty.tomorrow",
  upcoming: "confirmations.list.empty.upcoming",
  all: "confirmations.list.empty.all",
};
const FILTER_IDS = ["state", "branch"] as const;
const SORT_FIELDS = ["starts_at", "deadline_at", "number", "customer"];

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

function viewOf(raw: string | null): View {
  return VIEWS.includes(raw as View) ? (raw as View) : DEFAULT_VIEW;
}

/**
 * Teyit listesi (TEC-924, the confirmation add-on): the bookings of the
 * day (or the next days) with the business's approval and the customer's
 * confirmation side by side, the requests and their results, the answers,
 * and what needs a person before the visit (a change request, no answer,
 * no way to reach the customer, a request that failed on every channel).
 * Quick actions: call, "confirmed on the phone", move, cancel, open.
 */
export function ConfirmationListPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { hasModule } = useSector();
  const enabled = hasModule("confirmation");

  return (
    <EntityPage
      title={t("confirmations.list.title")}
      description={t("confirmations.list.description")}
      permission={permissions.tenant.reservations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("confirmations.list.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("confirmations.list.title") },
      ]}
    >
      {enabled ? (
        <ConfirmationList />
      ) : (
        <EmptyState
          title={t("confirmations.addon_off.title")}
          description={t("confirmations.addon_off.description")}
          action={
            <Button asChild variant="outline" size="sm">
              <Link href={routes.tenant.settings.plan(slug)}>
                {t("confirmations.addon_off.action")}
              </Link>
            </Button>
          }
        />
      )}
    </EntityPage>
  );
}

function ConfirmationList() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const search = useSearchParams();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.reservations.write);
  const zone = organization?.timezone ?? "Europe/Istanbul";

  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "starts_at",
    initialPageSize: 20,
    initialColumnFilters: urlDefaults.initialColumnFilters,
    realtime: tenantLiveList(organization, RESERVATIONS_RESOURCE),
  });

  // Fixed at the choice, so the query key stays stable.
  const [view, setView] = useState<{ value: View; at: string }>(() => ({
    value: viewOf(search.get("view")),
    at: new Date().toISOString(),
  }));
  const [selected, setSelected] = useState<ConfirmationTask | null>(null);
  const [phoneTarget, setPhoneTarget] = useState<PhoneConfirmTarget | null>(
    null,
  );
  const [actionTarget, setActionTarget] = useState<ActionTarget | null>(null);
  const [rescheduleUuid, setRescheduleUuid] = useState<string | null>(null);

  const changeView = useCallback(
    (value: View) => {
      setView({ value, at: new Date().toISOString() });
      listState.tableState.onPaginationChange?.((prev) => ({
        ...prev,
        pageIndex: 0,
      }));
    },
    [listState.tableState],
  );

  const range = useMemo(() => {
    const at = new Date(view.at);
    return view.value === "attention"
      ? { starts_from: view.at }
      : periodRange(view.value, at, zone);
  }, [view, zone]);

  const listParams = useMemo<ListConfirmationsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      state: joined(column("state")),
      branch: joined(column("branch")),
      attention: view.value === "attention" ? "true" : undefined,
      ...range,
    };
  }, [listState.columnFilters, listState.params, range, view.value]);

  const listQuery = useConfirmationList(listParams);
  const facetsQuery = useConfirmationFacets({
    q: listParams.q,
    branch: listParams.branch,
    starts_from: listParams.starts_from,
    starts_to: listParams.starts_to,
  });
  const summaryQuery = useConfirmationSummary();
  const branchesQuery = useAccessibleBranches();

  const handlers = useMemo(
    () => ({
      open: (row: ConfirmationTask) =>
        router.push(routes.tenant.reservations.detail(slug, row.uuid)),
      details: (row: ConfirmationTask) => setSelected(row),
      confirm: (row: ConfirmationTask) =>
        setPhoneTarget({
          uuid: row.uuid,
          number: row.number,
          version: row.version,
          customerName: row.customer.name,
          phone: row.customer.phone,
        }),
      reschedule: (row: ConfirmationTask) => setRescheduleUuid(row.uuid),
      cancel: (row: ConfirmationTask) =>
        setActionTarget({
          uuid: row.uuid,
          number: row.number,
          version: row.version,
          action: "cancel",
        }),
    }),
    [router, slug],
  );

  const stateCounts = facetsQuery.data?.state;
  const branches = branchesQuery.data?.items;
  const columns = useMemo(() => {
    const when = (iso: string | null | undefined) =>
      iso
        ? datetimeInZone(iso, zone, locale, {
            dateStyle: "short",
            timeStyle: "short",
          })
        : "—";
    const stateOptions = (Object.keys(STATES) as ConfirmationState[]).map(
      (state) => {
        const count = stateCounts?.find((f) => f.value === state)?.count;
        return {
          value: state,
          label: `${t(STATES[state].key)} (${count ?? 0})`,
        };
      },
    );
    const branchOptions = (branches ?? []).map((branch) => ({
      value: branch.uuid,
      label: branch.name,
    }));
    return [
      createColumn<ConfirmationTask>({
        id: "customer",
        accessorFn: (row) => row.customer.name,
        labelKey: "confirmations.list.columns.customer",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex flex-col">
            <span className="flex items-center gap-1.5">
              <Link
                href={routes.tenant.reservations.detail(
                  slug,
                  row.original.uuid,
                )}
                className="font-medium hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                #{row.original.number}
              </Link>
              <span className="font-medium">{row.original.customer.name}</span>
            </span>
            {row.original.customer.phone ? (
              <a
                href={telHref(row.original.customer.phone) ?? undefined}
                className="text-muted-foreground text-xs hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {row.original.customer.phone}
              </a>
            ) : null}
          </span>
        ),
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "starts_at",
        labelKey: "confirmations.list.columns.starts_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="tabular-nums">{when(row.original.starts_at)}</span>
        ),
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "services",
        labelKey: "confirmations.list.columns.services",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="line-clamp-2">
            {[row.original.services, row.original.staff]
              .filter(Boolean)
              .join(" · ") || "—"}
          </span>
        ),
      }),
      createColumn<ConfirmationTask>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "confirmations.list.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: branchOptions,
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "approval_status",
        labelKey: "confirmations.list.columns.approval_status",
        enableSorting: false,
        cell: ({ row }) => (
          <ApprovalChip status={row.original.approval_status} />
        ),
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "state",
        labelKey: "confirmations.list.columns.state",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: stateOptions,
        cell: ({ row }) => (
          <span className="flex flex-wrap items-center gap-1">
            <StatusChip
              label={t(STATES[row.original.state].key)}
              tone={STATES[row.original.state].tone}
            />
            {row.original.attention ? (
              <Badge variant="warning">
                {t("confirmations.list.attention_badge")}
              </Badge>
            ) : null}
          </span>
        ),
      }),
      createColumn<ConfirmationTask>({
        id: "attempts",
        accessorFn: (row) => row.attempts_sent,
        labelKey: "confirmations.list.columns.attempts",
        enableSorting: false,
        cell: ({ row }) => {
          const r = row.original;
          const last = r.last_send ? SEND_STATUS[r.last_send.status] : null;
          return (
            <span className="flex flex-col gap-1">
              <span className="tabular-nums">
                {t("reservations.confirmation_task.attempts_value", {
                  sent: r.attempts_sent,
                  planned: r.attempts_planned,
                })}
              </span>
              <span className="text-muted-foreground text-xs">
                {r.channels.map((ch) => channelLabel(t, ch)).join(" → ")}
              </span>
              {last ? (
                <StatusChip label={t(last.key)} tone={last.tone} />
              ) : null}
            </span>
          );
        },
      }),
      createColumn<ConfirmationTask>({
        id: "answer",
        accessorFn: (row) => row.answer ?? "",
        labelKey: "confirmations.list.columns.answer",
        enableSorting: false,
        cell: ({ row }) => {
          const r = row.original;
          if (!r.answer) {
            return <span className="text-muted-foreground">—</span>;
          }
          return (
            <span className="flex flex-col">
              <span>{t(ANSWER_KEYS[r.answer] ?? r.answer)}</span>
              <span className="text-muted-foreground text-xs">
                {[
                  r.answer_channel
                    ? t(ANSWER_SOURCES[r.answer_channel] ?? r.answer_channel)
                    : "",
                  when(r.answered_at),
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </span>
              {r.change_note ? (
                <span className="line-clamp-2 text-xs">{r.change_note}</span>
              ) : null}
            </span>
          );
        },
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "deadline_at",
        labelKey: "confirmations.list.columns.deadline_at",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="tabular-nums">{when(row.original.deadline_at)}</span>
        ),
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "next_attempt_at",
        labelKey: "confirmations.list.columns.next_attempt_at",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {when(row.original.next_attempt_at)}
          </span>
        ),
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "status",
        labelKey: "confirmations.list.columns.status",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <ReservationStatusChip status={row.original.status} />
        ),
      }),
      createColumn<ConfirmationTask>({
        accessorKey: "updated_at",
        labelKey: "confirmations.list.columns.updated_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">{when(row.original.updated_at)}</span>
        ),
      }),
      createColumn<ConfirmationTask>({
        id: "actions",
        labelKey: "confirmations.list.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const r = row.original;
          const tel = telHref(r.customer.phone);
          const actions: EntityRowAction[] = [
            {
              id: "details",
              label: t("confirmations.actions.details"),
              icon: ListChecks,
              onSelect: () => handlers.details(r),
            },
            {
              id: "open",
              label: t("confirmations.actions.open"),
              icon: Eye,
              onSelect: () => handlers.open(r),
            },
          ];
          if (tel) {
            actions.push({
              id: "call",
              label: t("confirmations.actions.call"),
              icon: Phone,
              onSelect: () => {
                window.location.href = tel;
              },
            });
          }
          if (canWrite && r.can_confirm) {
            actions.push({
              id: "confirm",
              label: t("confirmations.phone.action"),
              icon: ShieldCheck,
              onSelect: () => handlers.confirm(r),
            });
          }
          if (canWrite && isActiveVisit(r)) {
            actions.push(
              {
                id: "reschedule",
                label: t("confirmations.actions.reschedule"),
                icon: CalendarClock,
                onSelect: () => handlers.reschedule(r),
              },
              {
                id: "cancel",
                label: t("confirmations.actions.cancel"),
                icon: Ban,
                variant: "destructive",
                separatorBefore: true,
                onSelect: () => handlers.cancel(r),
              },
            );
          }
          return <EntityRowActions actions={actions} />;
        },
      }),
    ] as ColumnDef<ConfirmationTask, unknown>[];
  }, [branches, canWrite, handlers, locale, slug, stateCounts, t, zone]);

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.state) ||
    Boolean(listParams.branch);

  const refresh = () =>
    Promise.all([
      listQuery.refetch(),
      facetsQuery.refetch(),
      summaryQuery.refetch(),
    ]);

  // The drawer shows the row as the list has it now (live updates).
  const current = selected
    ? (listQuery.data?.items.find((row) => row.uuid === selected.uuid) ??
      selected)
    : null;

  return (
    <div className="space-y-4">
      {view.value !== "attention" ? (
        <ConfirmationAlerts onShow={() => changeView("attention")} />
      ) : null}
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={handlers.details}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("confirmations.list.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(refresh)}
        emptyTitle={
          hasFilters
            ? t("confirmations.list.empty_filtered_title")
            : t(EMPTY_KEYS[view.value])
        }
        emptyDescription={
          hasFilters
            ? t("confirmations.list.empty_filtered_description")
            : t("confirmations.list.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-confirmations-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <Select
              value={view.value}
              onValueChange={(value) => changeView(value as View)}
            >
              <SelectTrigger
                size="sm"
                className="w-44"
                aria-label={t("confirmations.list.view.label")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {VIEWS.map((value) => (
                  <SelectItem key={value} value={value}>
                    {value === "attention"
                      ? t("confirmations.list.view.attention", {
                          count: summaryQuery.data?.attention ?? 0,
                        })
                      : t(VIEW_KEYS[value])}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <EntityToolbar
              onRefresh={() => void refresh()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <ConfirmationDrawer
        row={current}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
        actions={{
          canWrite,
          onConfirm: handlers.confirm,
          onReschedule: handlers.reschedule,
          onCancel: handlers.cancel,
        }}
      />
      <ConfirmByPhoneDialog
        target={phoneTarget}
        onOpenChange={(open) => {
          if (!open) setPhoneTarget(null);
        }}
      />
      <ReservationActionDialog
        target={actionTarget}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
      />
      <RescheduleDialog
        uuid={rescheduleUuid}
        onOpenChange={(open) => {
          if (!open) setRescheduleUuid(null);
        }}
      />
    </div>
  );
}

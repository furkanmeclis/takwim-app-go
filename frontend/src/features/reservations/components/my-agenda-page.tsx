"use client";

import {
  CalendarClock,
  ChevronLeft,
  ChevronRight,
  Eye,
  RefreshCw,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ReservationActionDialog,
  type ActionTarget,
} from "@/features/reservations/components/reservation-action-dialog";
import { ACTION_ICONS } from "@/features/reservations/components/reservation-row-actions";
import { ReservationStatusChip } from "@/features/reservations/components/reservation-status-chips";
import { RescheduleDialog } from "@/features/reservations/components/reschedule-dialog";
import {
  ACTION_LABEL_KEYS,
  RESERVATIONS_RESOURCE,
  allowedActions,
  canReschedule,
} from "@/features/reservations/constants";
import { useReservationAgenda } from "@/features/reservations/hooks/use-reservations";
import { addDays } from "@/features/reservations/lib/calendar-time";
import {
  READ_ANY,
  useCanWriteReservations,
} from "@/features/reservations/lib/permissions";
import type { AgendaItem } from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import { useChannel } from "@/hooks/use-realtime";
import { dateKeyInZone, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * The signed-in member's own day (TEC-902): when it works in which branch,
 * its leave, and the visits it serves with the customer's phone and note.
 * Status changes and moves go through the same dialogs as the list; the API
 * refuses a change to a visit shared with a colleague.
 */
export function MyAgendaPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { canAny } = usePermission();
  const canWrite = useCanWriteReservations();
  const zone = organization?.timezone ?? "Europe/Istanbul";

  const [date, setDate] = useState(() => dateKeyInZone(new Date(), zone));
  const [includeCancelled, setIncludeCancelled] = useState(false);
  const [actionTarget, setActionTarget] = useState<ActionTarget | null>(null);
  const [rescheduleUuid, setRescheduleUuid] = useState<string | null>(null);

  const query = useReservationAgenda({
    from: date,
    to: date,
    include_cancelled: includeCancelled ? "true" : undefined,
  });
  const agenda = query.data;
  const timezone = agenda?.timezone ?? zone;

  useChannel(
    organization?.uuid &&
      canAny([
        permissions.tenant.reservations.read,
        permissions.tenant.reservations.own,
      ])
      ? tenantResourceChannel(organization.uuid, RESERVATIONS_RESOURCE)
      : null,
    () => void query.refetch(),
  );

  const time = (value: string) =>
    datetimeInZone(value, timezone, locale, { timeStyle: "short" });

  const columns = useMemo(
    () => [
      createColumn<AgendaItem>({
        accessorKey: "starts_at",
        labelKey: "reservations.agenda.time",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-medium tabular-nums">
            {datetimeInZone(row.original.starts_at, timezone, locale, {
              timeStyle: "short",
            })}
            –
            {datetimeInZone(row.original.ends_at, timezone, locale, {
              timeStyle: "short",
            })}
          </span>
        ),
      }),
      createColumn<AgendaItem>({
        accessorKey: "customer_name",
        labelKey: "reservations.fields.customer",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="font-medium">
              {row.original.customer_name} · #{row.original.number}
            </span>
            {row.original.customer_phone ? (
              <a
                href={`tel:${row.original.customer_phone}`}
                className="text-muted-foreground text-xs tabular-nums hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {row.original.customer_phone}
              </a>
            ) : null}
          </div>
        ),
      }),
      createColumn<AgendaItem>({
        accessorKey: "service_name",
        labelKey: "reservations.lines.service",
        enableSorting: false,
      }),
      createColumn<AgendaItem>({
        accessorKey: "branch_name",
        labelKey: "reservations.fields.branch",
        enableSorting: false,
      }),
      createColumn<AgendaItem>({
        accessorKey: "status",
        labelKey: "reservations.columns.status",
        enableSorting: false,
        cell: ({ row }) => (
          <ReservationStatusChip status={row.original.status} />
        ),
      }),
      createColumn<AgendaItem>({
        accessorKey: "customer_note",
        labelKey: "reservations.agenda.note",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.customer_note ? (
            <span className="text-muted-foreground line-clamp-2 text-sm">
              {row.original.customer_note}
            </span>
          ) : null,
      }),
      createColumn<AgendaItem>({
        id: "actions",
        labelKey: "reservations.columns.actions",
        enableSorting: false,
        cell: ({ row }) => {
          const item = row.original;
          const actions: EntityRowAction[] = [
            {
              id: "view",
              label: t("reservations.actions.view"),
              icon: Eye,
              onSelect: () =>
                router.push(
                  routes.tenant.reservations.detail(
                    slug,
                    item.reservation_uuid,
                  ),
                ),
            },
          ];
          if (canWrite) {
            if (canReschedule(item.status)) {
              actions.push({
                id: "reschedule",
                label: t("reservations.actions.reschedule"),
                icon: CalendarClock,
                onSelect: () => setRescheduleUuid(item.reservation_uuid),
              });
            }
            for (const action of allowedActions(
              item.status,
              item.approval_status,
            )) {
              actions.push({
                id: action,
                label: t(ACTION_LABEL_KEYS[action]),
                icon: ACTION_ICONS[action],
                variant:
                  action === "cancel" ||
                  action === "reject" ||
                  action === "no-show"
                    ? "destructive"
                    : "default",
                separatorBefore: action === "cancel",
                onSelect: () =>
                  setActionTarget({
                    uuid: item.reservation_uuid,
                    number: item.number,
                    version: item.version,
                    action,
                  }),
              });
            }
          }
          return <EntityRowActions actions={actions} />;
        },
      }),
    ],
    [canWrite, locale, router, slug, t, timezone],
  );

  return (
    <EntityPage
      title={t("reservations.agenda.title")}
      description={t("reservations.agenda.description")}
      permission={READ_ANY}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reservations.agenda.title") },
      ]}
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={t("reservations.calendar.previous")}
              onClick={() => setDate((d) => addDays(d, -1))}
            >
              <ChevronLeft className="size-4" />
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDate(dateKeyInZone(new Date(), zone))}
            >
              {t("reservations.calendar.today")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={t("reservations.calendar.next")}
              onClick={() => setDate((d) => addDays(d, 1))}
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
          <DatePicker
            className="w-44"
            value={date}
            onChange={(next) => next && setDate(next)}
          />
          <div className="flex items-center gap-2">
            <Switch
              id="agenda-cancelled"
              checked={includeCancelled}
              onCheckedChange={setIncludeCancelled}
            />
            <Label htmlFor="agenda-cancelled">
              {t("reservations.calendar.show_cancelled")}
            </Label>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="ms-auto"
            aria-label={t("reservations.calendar.refresh")}
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw
              className={query.isFetching ? "size-4 animate-spin" : "size-4"}
            />
          </Button>
        </div>

        {query.isLoading ? <Loading label={t("common.loading")} /> : null}
        {query.isError ? (
          <ErrorState
            title={t("reservations.agenda.load_failed")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {agenda && !agenda.staff ? (
          <EmptyState
            title={t("reservations.agenda.no_staff_title")}
            description={t("reservations.agenda.no_staff_description")}
          />
        ) : null}
        {agenda?.staff ? (
          <>
            <EntitySectionCard title={t("reservations.agenda.shift")}>
              {agenda.work.length === 0 && agenda.off.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("reservations.agenda.no_shift")}
                </p>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {agenda.work.map((w) => (
                    <StatusChip
                      key={`${w.branch.uuid}-${w.starts_at}`}
                      tone="success"
                      label={`${w.branch.name} · ${time(w.starts_at)}–${time(w.ends_at)}`}
                    />
                  ))}
                  {agenda.off.map((o) => (
                    <StatusChip
                      key={`off-${o.starts_at}`}
                      tone="warning"
                      label={`${t("reservations.agenda.leave")} · ${time(o.starts_at)}–${time(o.ends_at)}`}
                    />
                  ))}
                </div>
              )}
            </EntitySectionCard>
            <EntitySectionCard
              title={t("reservations.agenda.visits")}
              badge={agenda.items.length}
            >
              <EntityTable
                columns={columns}
                data={agenda.items}
                getRowId={(row) => `${row.reservation_uuid}-${row.line_index}`}
                onRowClick={(row) =>
                  router.push(
                    routes.tenant.reservations.detail(
                      slug,
                      row.reservation_uuid,
                    ),
                  )
                }
                emptyTitle={t("reservations.agenda.empty_title")}
                emptyDescription={t("reservations.agenda.empty_description")}
                manual={{ filtering: false, sorting: false, pagination: false }}
                features={{
                  rowSelection: false,
                  columnFilters: false,
                  facetedFilters: false,
                  globalFilter: false,
                  sorting: false,
                  pagination: false,
                  persistKey: "tenant-reservation-agenda-v1",
                }}
              />
            </EntitySectionCard>
          </>
        ) : null}
      </div>

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
    </EntityPage>
  );
}

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  Check,
  ExternalLink,
  RotateCcw,
  UserX,
  Users,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  SessionChangeDialog,
  type SessionChangeMode,
} from "@/features/classes/components/session-change-dialog";
import { useSessionWhen } from "@/features/classes/components/sessions-table";
import {
  SESSION_STATE_LABEL_KEYS,
  SESSION_STATE_TONES,
} from "@/features/classes/constants";
import {
  useChangeClassSession,
  useClassSession,
} from "@/features/classes/hooks/use-classes";
import type { ClassAttendee } from "@/features/classes/types";
import { PAYMENT, STATUS } from "@/features/reservations/constants";
import { useReservationAction } from "@/features/reservations/hooks/use-reservations";
import type {
  PaymentStatus,
  ReservationStatus,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Attendance is open for visits that are booked or in progress. */
const MARKABLE: ReservationStatus[] = ["active", "in_service"];

/**
 * One session of a class: seats, the bookings in it and attendance.
 * "Attended" completes the visit and "no-show" marks it, the visit's own
 * steps, so payments, packages and reports follow as for any visit.
 */
export function SessionDetailPage({
  uuid,
  date,
}: {
  uuid: string;
  date: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const when = useSessionWhen();
  const query = useClassSession(uuid, date);
  const action = useReservationAction();
  const change = useChangeClassSession();
  const [mode, setMode] = useState<SessionChangeMode | null>(null);
  const [restoring, setRestoring] = useState(false);
  const session = query.data;
  const canWrite = can(permissions.tenant.classes.write);
  const canMark = can(permissions.tenant.reservations.write);
  const editable = canWrite && session != null && session.state !== "past";
  const title = session?.service.name ?? t("classes.session.title");
  const w = session ? when(session) : null;

  const columns = useMemo(
    () =>
      [
        createColumn<ClassAttendee>({
          id: "customer",
          accessorFn: (row) => row.customer.name,
          labelKey: "classes.session.columns.customer",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <Link
                className="text-primary truncate font-medium hover:underline"
                href={routes.tenant.customers.detail(
                  slug,
                  row.original.customer.uuid,
                )}
                onClick={(event) => event.stopPropagation()}
              >
                {row.original.customer.name}
              </Link>
              {row.original.customer.phone ? (
                <p className="text-muted-foreground text-xs tabular-nums">
                  {row.original.customer.phone}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<ClassAttendee>({
          accessorKey: "number",
          labelKey: "classes.session.columns.number",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">#{row.original.number}</span>
          ),
        }),
        createColumn<ClassAttendee>({
          accessorKey: "seats",
          labelKey: "classes.session.columns.seats",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.seats}</span>
          ),
        }),
        createColumn<ClassAttendee>({
          accessorKey: "status",
          labelKey: "classes.session.columns.status",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const status = STATUS[row.original.status as ReservationStatus];
            return status ? (
              <StatusChip label={t(status.key)} tone={status.tone} />
            ) : (
              row.original.status
            );
          },
        }),
        createColumn<ClassAttendee>({
          accessorKey: "payment_status",
          labelKey: "classes.session.columns.payment_status",
          enableSorting: false,
          cell: ({ row }) => {
            const payment =
              PAYMENT[row.original.payment_status as PaymentStatus];
            return payment ? (
              <StatusChip label={t(payment.key)} tone={payment.tone} />
            ) : (
              row.original.payment_status
            );
          },
        }),
        createColumn<ClassAttendee>({
          id: "actions",
          labelKey: "classes.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const a = row.original;
            const markable =
              canMark && MARKABLE.includes(a.status as ReservationStatus);
            return (
              <EntityRowActions
                actions={[
                  ...(markable
                    ? [
                        {
                          id: "attended",
                          label: t("classes.actions.attended"),
                          icon: Check,
                          permission: permissions.tenant.reservations.write,
                          onSelect: () =>
                            action.mutate({
                              uuid: a.reservation_uuid,
                              action: "complete",
                              body: {},
                            }),
                        },
                        {
                          id: "no-show",
                          label: t("classes.actions.no_show"),
                          icon: UserX,
                          permission: permissions.tenant.reservations.write,
                          variant: "destructive" as const,
                          onSelect: () =>
                            action.mutate({
                              uuid: a.reservation_uuid,
                              action: "no-show",
                              body: {},
                            }),
                        },
                      ]
                    : []),
                  {
                    id: "open",
                    label: t("classes.actions.open_reservation"),
                    icon: ExternalLink,
                    onSelect: () =>
                      router.push(
                        routes.tenant.reservations.detail(
                          slug,
                          a.reservation_uuid,
                        ),
                      ),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ClassAttendee, unknown>[],
    [action, canMark, router, slug, t],
  );

  const attendees = session?.attendees ?? [];
  const attended = attendees.filter((a) => a.status === "completed").length;
  const noShow = attendees.filter((a) => a.status === "no_show").length;

  return (
    <EntityPage
      title={title}
      description={t("classes.session.description")}
      permission={permissions.tenant.classes.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("classes.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("classes.title"), href: routes.tenant.classes.root(slug) },
        { label: w ? `${title} · ${w.day}` : title },
      ]}
      actions={
        editable ? (
          <EntityActions>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setMode("seats")}
            >
              <Users className="size-4" />
              {t("classes.actions.change_seats")}
            </Button>
            {session.cancelled ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setRestoring(true)}
              >
                <RotateCcw className="size-4" />
                {t("classes.actions.restore_session")}
              </Button>
            ) : (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setMode("cancel")}
              >
                <Ban className="size-4" />
                {t("classes.actions.cancel_session")}
              </Button>
            )}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("classes.session.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {session && w ? (
        <div className="space-y-6">
          <EntityHeader
            title={session.service.name}
            subtitle={`${w.day} · ${w.time} · ${session.branch.name}`}
            badges={
              <StatusChip
                label={t(SESSION_STATE_LABEL_KEYS[session.state])}
                tone={SESSION_STATE_TONES[session.state]}
              />
            }
          />

          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatsCard
              title={t("classes.session.booked")}
              value={t("classes.sessions.booked_value", {
                booked: session.booked,
                capacity: session.capacity,
              })}
              hint={
                session.capacity_changed
                  ? t("classes.sessions.capacity_changed")
                  : undefined
              }
            />
            <StatsCard
              title={t("classes.session.seats_left")}
              value={String(session.seats_left)}
            />
            <StatsCard
              title={t("classes.session.attended")}
              value={String(attended)}
            />
            <StatsCard
              title={t("classes.session.no_show")}
              value={String(noShow)}
            />
          </div>

          <EntitySectionCard title={t("classes.session.details")}>
            <EntityDetail
              sections={[
                {
                  id: "session",
                  fields: [
                    {
                      key: "branch",
                      label: t("classes.columns.branch"),
                      value: session.branch.name,
                    },
                    {
                      key: "staff",
                      label: t("classes.columns.staff"),
                      value:
                        session.staff?.name ?? t("classes.picked_on_booking"),
                    },
                    {
                      key: "resource",
                      label: t("classes.columns.resource"),
                      value: session.resource?.name ?? "—",
                    },
                    {
                      key: "reason",
                      label: t("classes.fields.reason"),
                      value: session.reason || "—",
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("classes.session.attendees")}
            badge={attendees.length}
          >
            <p className="text-muted-foreground mb-3 text-sm">
              {t("classes.session.attendees_hint")}
            </p>
            <EntityTable
              columns={columns}
              data={attendees}
              getRowId={(row) => row.reservation_uuid}
              onRowClick={(row) =>
                router.push(
                  routes.tenant.reservations.detail(slug, row.reservation_uuid),
                )
              }
              emptyTitle={t("classes.session.attendees_empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
                globalFilter: false,
                sorting: false,
                pagination: false,
                viewMode: true,
                persistKey: "tenant-class-session-attendees-v1",
              }}
            />
          </EntitySectionCard>

          {mode ? (
            <SessionChangeDialog
              session={session}
              mode={mode}
              open
              onOpenChange={(open) => {
                if (!open) setMode(null);
              }}
            />
          ) : null}
          <ConfirmDialog
            open={restoring}
            title={t("classes.session_dialog.restore_title")}
            description={t("classes.session_dialog.restore_description")}
            confirmLabel={t("classes.actions.restore_session")}
            isPending={change.isPending}
            onCancel={() => setRestoring(false)}
            onConfirm={() =>
              change.mutate(
                {
                  uuid: session.schedule_uuid,
                  date: session.date,
                  body: {
                    cancelled: false,
                    capacity: session.capacity_changed
                      ? session.capacity
                      : null,
                    reason: "",
                  },
                },
                { onSettled: () => setRestoring(false) },
              )
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}

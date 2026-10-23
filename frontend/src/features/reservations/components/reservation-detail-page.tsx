"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CalendarClock, Hourglass } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { AppForm, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ReservationActionDialog,
  type ActionTarget,
} from "@/features/reservations/components/reservation-action-dialog";
import { ACTION_ICONS } from "@/features/reservations/components/reservation-row-actions";
import {
  ApprovalChip,
  ConfirmationChip,
  PaymentChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import { RescheduleDialog } from "@/features/reservations/components/reschedule-dialog";
import {
  ACTION_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  allowedActions,
  canReschedule,
} from "@/features/reservations/constants";
import {
  useReservation,
  useUpdateReservationNotes,
} from "@/features/reservations/hooks/use-reservations";
import {
  cancelReasonLabel,
  partyLabel,
  sourceLabel,
  statusLabel,
} from "@/features/reservations/lib/labels";
import {
  READ_ANY,
  useCanWriteReservations,
} from "@/features/reservations/lib/permissions";
import {
  createNotesSchema,
  type NotesFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import type {
  Reservation,
  ReservationLine,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { amount, datetime, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Show page: visit, lines, notes, history; status changes and moving. */
export function ReservationDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  const { can } = usePermission();
  const canWrite = useCanWriteReservations();
  const query = useReservation(uuid);
  const reservation = query.data;
  const [actionTarget, setActionTarget] = useState<ActionTarget | null>(null);
  const [rescheduling, setRescheduling] = useState(false);

  const title = reservation
    ? `#${reservation.number} · ${reservation.customer.name}`
    : t("reservations.detail_title");
  const when = (iso: string, options?: Intl.DateTimeFormatOptions) =>
    datetimeInZone(
      iso,
      zone,
      locale,
      options ?? { dateStyle: "medium", timeStyle: "short" },
    );

  return (
    <EntityPage
      title={title}
      description={t("reservations.detail_description")}
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
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: reservation ? `#${reservation.number}` : title },
      ]}
      actions={
        reservation && canWrite ? (
          <EntityActions>
            {canReschedule(reservation.status) ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setRescheduling(true)}
              >
                <CalendarClock className="size-4" />
                {t("reservations.actions.reschedule")}
              </Button>
            ) : null}
            {allowedActions(
              reservation.status,
              reservation.approval_status,
            ).map((action) => {
              const Icon = ACTION_ICONS[action];
              const destructive =
                action === "cancel" ||
                action === "reject" ||
                action === "no-show";
              return (
                <Button
                  key={action}
                  type="button"
                  size="sm"
                  variant={destructive ? "outline" : "default"}
                  className={
                    destructive
                      ? "text-destructive hover:text-destructive"
                      : undefined
                  }
                  onClick={() =>
                    setActionTarget({
                      uuid: reservation.uuid,
                      number: reservation.number,
                      version: reservation.version,
                      action,
                    })
                  }
                >
                  <Icon className="size-4" />
                  {t(ACTION_LABEL_KEYS[action])}
                </Button>
              );
            })}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("reservations.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {reservation ? (
        <div className="space-y-6">
          <EntityHeader
            title={title}
            subtitle={`${when(reservation.starts_at)} – ${when(
              reservation.ends_at,
              { timeStyle: "short" },
            )} · ${reservation.branch.name}`}
            badges={
              <>
                <ReservationStatusChip status={reservation.status} />
                <ApprovalChip status={reservation.approval_status} />
                <ConfirmationChip status={reservation.confirmation_status} />
                <PaymentChip status={reservation.payment_status} />
              </>
            }
          />

          {reservation.status === "requested" && reservation.hold_expires_at ? (
            <Alert>
              <Hourglass className="size-4" />
              <AlertDescription>
                {t("reservations.detail.hold_expires", {
                  at: when(reservation.hold_expires_at),
                })}
              </AlertDescription>
            </Alert>
          ) : null}

          <EntitySectionCard title={t("reservations.detail.visit")}>
            <EntityDetail
              sections={[
                {
                  id: "visit",
                  fields: [
                    {
                      key: "customer",
                      label: t("reservations.fields.customer"),
                      value: can(permissions.tenant.customers.read) ? (
                        <Link
                          className="text-primary hover:underline"
                          href={routes.tenant.customers.detail(
                            slug,
                            reservation.customer.uuid,
                          )}
                        >
                          {reservation.customer.name}
                        </Link>
                      ) : (
                        reservation.customer.name
                      ),
                    },
                    {
                      key: "branch",
                      label: t("reservations.fields.branch"),
                      value: reservation.branch.name,
                    },
                    ...(reservation.party_size != null || reservation.zone
                      ? [
                          {
                            key: "party",
                            label: t("reservations.columns.party"),
                            value: partyLabel(t, reservation),
                          },
                        ]
                      : []),
                    {
                      key: "starts_at",
                      label: t("reservations.columns.starts_at"),
                      value: when(reservation.starts_at),
                    },
                    {
                      key: "ends_at",
                      label: t("reservations.fields.ends_at"),
                      value: when(reservation.ends_at),
                    },
                    {
                      key: "total_price",
                      label: t("reservations.columns.total_price"),
                      value: amount(
                        reservation.total_price,
                        reservation.currency,
                        locale,
                      ),
                    },
                    {
                      key: "source",
                      label: t("reservations.columns.source"),
                      value: sourceLabel(t, reservation.source),
                    },
                    ...(reservation.cancel_reason
                      ? [
                          {
                            key: "cancel_reason",
                            label: t("reservations.fields.cancel_reason"),
                            value: cancelReasonLabel(
                              t,
                              reservation.cancel_reason,
                            ),
                          },
                        ]
                      : []),
                    {
                      key: "created_at",
                      label: t("reservations.columns.created_at"),
                      value: datetime(
                        reservation.created_at,
                        undefined,
                        locale,
                      ),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <ReservationLinesCard reservation={reservation} />

          <ReservationNotesCard reservation={reservation} editable={canWrite} />

          <EntitySectionCard
            title={t("reservations.detail.history")}
            badge={reservation.events.length}
          >
            <Timeline
              items={[...reservation.events].reverse().map((event) => ({
                id: `${event.version}-${event.type}`,
                title: t(EVENT_LABEL_KEYS[event.type] ?? event.type),
                description: [
                  event.to_status && event.from_status !== event.to_status
                    ? statusLabel(t, event.to_status)
                    : "",
                  event.actor ?? "",
                  event.note,
                ]
                  .filter(Boolean)
                  .join(" · "),
                meta: datetime(event.created_at, undefined, locale),
              }))}
            />
          </EntitySectionCard>
        </div>
      ) : null}

      <ReservationActionDialog
        target={actionTarget}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
      />
      <RescheduleDialog
        uuid={rescheduling ? uuid : null}
        onOpenChange={setRescheduling}
      />
    </EntityPage>
  );
}

function ReservationLinesCard({ reservation }: { reservation: Reservation }) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const zone = organization?.timezone ?? undefined;

  const columns = useMemo(
    () =>
      [
        createColumn<ReservationLine>({
          accessorKey: "starts_at",
          labelKey: "reservations.lines.time",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetimeInZone(row.original.starts_at, zone, locale, {
                timeStyle: "short",
              })}
              –
              {datetimeInZone(row.original.ends_at, zone, locale, {
                timeStyle: "short",
              })}
            </span>
          ),
        }),
        createColumn<ReservationLine>({
          accessorKey: "service_name",
          labelKey: "reservations.lines.service",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.service_name}</span>
          ),
        }),
        createColumn<ReservationLine>({
          id: "staff",
          accessorFn: (row) => row.staff?.name ?? "",
          labelKey: "reservations.lines.staff",
          enableSorting: false,
          cell: ({ row }) => row.original.staff?.name ?? "—",
        }),
        createColumn<ReservationLine>({
          id: "resources",
          accessorFn: (row) => row.resources.map((r) => r.name).join(", "),
          labelKey: "reservations.lines.resources",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.resources.map((r) => r.name).join(", ") || "—",
        }),
        createColumn<ReservationLine>({
          accessorKey: "duration_minutes",
          labelKey: "reservations.lines.duration",
          enableSorting: false,
          cell: ({ row }) =>
            t("services.minutes", {
              minutes: String(row.original.duration_minutes),
            }),
        }),
        createColumn<ReservationLine>({
          accessorKey: "unit_price",
          labelKey: "reservations.lines.price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.unit_price, reservation.currency, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<ReservationLine, unknown>[],
    [locale, reservation.currency, t, zone],
  );

  return (
    <EntitySectionCard
      title={t("reservations.detail.lines")}
      badge={reservation.lines.length}
    >
      <EntityTable
        columns={columns}
        data={reservation.lines}
        getRowId={(row) => String(row.index)}
        emptyTitle={t("reservations.lines.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-reservation-lines-v1",
        }}
      />
    </EntitySectionCard>
  );
}

function ReservationNotesCard({
  reservation,
  editable,
}: {
  reservation: Reservation;
  editable: boolean;
}) {
  const { t } = useLocale();
  const save = useUpdateReservationNotes();
  const schema = useMemo(() => createNotesSchema(t), [t]);

  if (!editable) {
    return (
      <EntitySectionCard title={t("reservations.detail.notes")}>
        <EntityDetail
          sections={[
            {
              id: "notes",
              fields: [
                {
                  key: "customer_note",
                  label: t("reservations.fields.customer_note"),
                  value: reservation.customer_note || "—",
                },
                {
                  key: "internal_note",
                  label: t("reservations.fields.internal_note"),
                  value: reservation.internal_note || "—",
                },
              ],
            },
          ]}
        />
      </EntitySectionCard>
    );
  }

  return (
    <EntitySectionCard title={t("reservations.detail.notes")}>
      <AppForm<NotesFormValues>
        key={reservation.version}
        className="grid gap-4"
        schema={schema}
        defaultValues={{
          customer_note: reservation.customer_note,
          internal_note: reservation.internal_note,
        }}
        onSubmit={async (values) => {
          await save.mutateAsync({
            uuid: reservation.uuid,
            body: { ...values, expected_version: reservation.version },
          });
        }}
      >
        {(form) => (
          <>
            <AppTextarea
              name="customer_note"
              label={t("reservations.fields.customer_note")}
              description={t("reservations.fields.customer_note_hint")}
            />
            <AppTextarea
              name="internal_note"
              label={t("reservations.fields.internal_note")}
              description={t("reservations.fields.internal_note_hint")}
            />
            <div className="flex justify-end">
              <Button
                type="submit"
                size="sm"
                disabled={!form.formState.isDirty || save.isPending}
              >
                {t("reservations.actions.save_notes")}
              </Button>
            </div>
          </>
        )}
      </AppForm>
    </EntitySectionCard>
  );
}

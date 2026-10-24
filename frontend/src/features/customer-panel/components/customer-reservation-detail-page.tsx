"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CalendarClock, Hourglass, XCircle } from "lucide-react";
import { type ReactNode, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Timeline } from "@/components/common/timeline";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { CustomerConfirmationCard } from "@/features/customer-panel/components/customer-confirmation-card";
import { CustomerPaymentsCard } from "@/features/customer-panel/components/customer-payments-card";
import { CustomerProductsCard } from "@/features/customer-panel/components/customer-products-card";
import { METHOD_KEYS } from "@/features/customer-panel/components/customer-reservation-columns";
import { CustomerRescheduleDialog } from "@/features/customer-panel/components/customer-reschedule-dialog";
import {
  useCancelReservation,
  useConfirmReservation,
  useCustomerReservation,
  useReloadReservation,
  useRequestReservationChange,
} from "@/features/customer-panel/hooks/use-customer-panel";
import {
  REASON_KEYS,
  customerErrorMessage,
  isConflict,
} from "@/features/customer-panel/lib/errors";
import type {
  CustomerAction,
  CustomerReservation,
} from "@/features/customer-panel/types";
import {
  ConfirmationChip,
  PaymentChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import { EVENT_LABEL_KEYS } from "@/features/reservations/constants";
import {
  cancelReasonLabel,
  statusLabel,
} from "@/features/reservations/lib/labels";
import { isApiError } from "@/lib/api";
import { amount, datetimeInZone, toMinor } from "@/lib/utils/format";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

type Line = CustomerReservation["lines"][number] & { index: number };

/**
 * `/panel/reservations/{uuid}`: one visit as the person sees it. The API
 * says whether it can still be cancelled or moved (`actions`) and until
 * when; the page only shows that answer.
 */
export function CustomerReservationDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const query = useCustomerReservation(uuid);
  const reservation = query.data;
  const cancel = useCancelReservation(uuid);
  const confirm = useConfirmReservation(uuid);
  const change = useRequestReservationChange(uuid);
  const reload = useReloadReservation(uuid);
  const [confirming, setConfirming] = useState(false);
  const [rescheduling, setRescheduling] = useState(false);

  const notFound =
    query.isError && isApiError(query.error) && query.error.status === 404;

  const title = reservation
    ? `${reservation.business.name} · #${reservation.number}`
    : t("customer_panel.detail.title");

  const runCancel = () =>
    reservation &&
    cancel.mutate(reservation.version, {
      onSuccess: () => {
        appToast.success(t("customer_panel.cancel.done"));
        setConfirming(false);
      },
      onError: (error) => {
        appToast.error(customerErrorMessage(t, error));
        setConfirming(false);
        if (isConflict(error)) void reload();
      },
    });

  const onAnswerError = (error: unknown) => {
    appToast.error(customerErrorMessage(t, error));
    if (isConflict(error)) void reload();
  };
  const runConfirm = () =>
    confirm.mutate(undefined, {
      onSuccess: () => appToast.success(t("customer_panel.confirmation.done")),
      onError: onAnswerError,
    });
  const runChange = (note: string) =>
    change
      .mutateAsync(note)
      .then(() => {
        appToast.success(t("customer_panel.confirmation.change_done"));
        return true;
      })
      .catch((error: unknown) => {
        onAnswerError(error);
        return false;
      });

  return (
    <EntityPage
      title={title}
      breadcrumbs={[
        {
          label: t("customer_panel.reservations.title"),
          href: routes.customer.root,
        },
        { label: reservation ? `#${reservation.number}` : "…" },
      ]}
      actions={
        reservation &&
        (reservation.actions.cancel.allowed ||
          reservation.actions.reschedule.allowed) ? (
          <EntityActions>
            {reservation.actions.reschedule.allowed ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setRescheduling(true)}
              >
                <CalendarClock className="size-4" />
                {t("customer_panel.actions.reschedule")}
              </Button>
            ) : null}
            {reservation.actions.cancel.allowed ? (
              <Button
                size="sm"
                variant="destructive"
                onClick={() => setConfirming(true)}
              >
                <XCircle className="size-4" />
                {t("customer_panel.actions.cancel")}
              </Button>
            ) : null}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={
            notFound
              ? t("customer_panel.detail.not_found")
              : t("customer_panel.errors.generic")
          }
          onRetry={notFound ? undefined : () => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {reservation ? (
        <div className="space-y-6">
          <CustomerReservationBody
            reservation={reservation}
            confirmation={
              <CustomerConfirmationCard
                reservation={reservation}
                confirming={confirm.isPending}
                requesting={change.isPending}
                onConfirm={runConfirm}
                onRequestChange={runChange}
              />
            }
          />

          <ConfirmDialog
            open={confirming}
            title={t("customer_panel.cancel.title")}
            description={t("customer_panel.cancel.description")}
            confirmLabel={t("customer_panel.actions.cancel")}
            cancelLabel={t("customer_panel.actions.keep")}
            variant="destructive"
            isPending={cancel.isPending}
            onConfirm={runCancel}
            onCancel={() => setConfirming(false)}
          />
          {reservation.actions.reschedule.allowed ? (
            <CustomerRescheduleDialog
              key={`${reservation.uuid}-${reservation.version}`}
              reservation={reservation}
              open={rescheduling}
              onOpenChange={setRescheduling}
            />
          ) : null}
        </div>
      ) : null}
    </EntityPage>
  );
}

/**
 * The visit as the person sees it: header, deadlines, place, lines,
 * payment, policies and history. The panel detail and the visit link page
 * (`/panel/v`) share it; each adds its own actions.
 */
export function CustomerReservationBody({
  reservation,
  confirmation,
}: {
  reservation: CustomerReservation;
  /** The confirmation request card, under the header (TEC-921). */
  confirmation?: ReactNode;
}) {
  const { t, locale } = useLocale();
  const zone = reservation.timezone;
  const when = (
    iso: string,
    options: Intl.DateTimeFormatOptions = {
      dateStyle: "medium",
      timeStyle: "short",
    },
  ) => datetimeInZone(iso, zone, locale, options);
  const cash = (value: string) =>
    amount(value, reservation.money.currency, locale);

  return (
    <>
      <EntityHeader
        title={reservation.services.join(", ") || reservation.business.name}
        subtitle={`${when(reservation.starts_at)} – ${when(
          reservation.ends_at,
          { timeStyle: "short" },
        )} · ${reservation.branch.name}`}
        badges={
          <>
            <ReservationStatusChip status={reservation.status} />
            {reservation.confirmation ? (
              <ConfirmationChip status={reservation.confirmation.status} />
            ) : null}
            <PaymentChip status={reservation.payment_status} />
          </>
        }
      />

      {confirmation}

      {reservation.status === "requested" ? (
        <Alert>
          <Hourglass className="size-4" />
          <AlertDescription>
            {reservation.hold_expires_at
              ? t("customer_panel.detail.awaiting_until", {
                  at: when(reservation.hold_expires_at),
                })
              : t("customer_panel.detail.awaiting")}
          </AlertDescription>
        </Alert>
      ) : null}

      <ActionNotes reservation={reservation} when={when} />

      <EntitySectionCard title={t("customer_panel.detail.visit")}>
        <EntityDetail
          sections={[
            {
              id: "visit",
              fields: [
                {
                  key: "business",
                  label: t("customer_panel.columns.business"),
                  value: reservation.business.name,
                },
                {
                  key: "branch",
                  label: t("customer_panel.detail.branch"),
                  value: reservation.branch.name,
                },
                {
                  key: "address",
                  label: t("customer_panel.detail.address"),
                  value:
                    [
                      reservation.branch.address,
                      reservation.branch.district,
                      reservation.branch.city,
                    ]
                      .filter(Boolean)
                      .join(", ") || "—",
                },
                {
                  key: "phone",
                  label: t("customer_panel.detail.phone"),
                  value: reservation.branch.phone ? (
                    <a
                      className="text-primary hover:underline"
                      href={`tel:${reservation.branch.phone}`}
                    >
                      {reservation.branch.phone}
                    </a>
                  ) : (
                    "—"
                  ),
                },
                {
                  key: "starts_at",
                  label: t("customer_panel.columns.starts_at"),
                  value: when(reservation.starts_at),
                },
                {
                  key: "ends_at",
                  label: t("customer_panel.detail.ends_at"),
                  value: when(reservation.ends_at),
                },
                ...(reservation.cancel_reason
                  ? [
                      {
                        key: "cancel_reason",
                        label: t("customer_panel.detail.cancel_reason"),
                        value: cancelReasonLabel(t, reservation.cancel_reason),
                      },
                    ]
                  : []),
                ...(reservation.customer_note
                  ? [
                      {
                        key: "note",
                        label: t("customer_panel.detail.note"),
                        value: reservation.customer_note,
                      },
                    ]
                  : []),
              ],
            },
          ]}
        />
      </EntitySectionCard>

      <LinesCard reservation={reservation} />

      <CustomerProductsCard reservation={reservation} />

      <EntitySectionCard title={t("customer_panel.detail.payment")}>
        <EntityDetail
          sections={[
            {
              id: "money",
              fields: [
                {
                  key: "total",
                  label: t("customer_panel.columns.total"),
                  value: amount(
                    reservation.money.total,
                    reservation.money.currency,
                    locale,
                  ),
                },
                {
                  key: "method",
                  label: t("customer_panel.columns.method"),
                  value: t(METHOD_KEYS[reservation.money.method]),
                },
                {
                  key: "expected",
                  label: t("customer_panel.columns.expected"),
                  value: cash(reservation.money.expected),
                },
                {
                  key: "paid",
                  label: t("customer_panel.columns.paid"),
                  value: cash(reservation.money.paid),
                },
                ...(toMinor(reservation.money.refunded)
                  ? [
                      {
                        key: "refunded",
                        label: t("customer_panel.columns.refunded"),
                        value: cash(reservation.money.refunded),
                      },
                    ]
                  : []),
                ...(toMinor(reservation.money.refund_pending)
                  ? [
                      {
                        key: "refund_pending",
                        label: t("customer_panel.columns.refund_pending"),
                        value: cash(reservation.money.refund_pending),
                      },
                    ]
                  : []),
                {
                  key: "balance",
                  label: t("customer_panel.columns.balance"),
                  value: cash(reservation.money.balance),
                },
              ],
            },
          ]}
        />
      </EntitySectionCard>

      <CustomerPaymentsCard reservation={reservation} />

      <RefundsCard reservation={reservation} />

      <PoliciesCard reservation={reservation} />

      <EntitySectionCard
        title={t("customer_panel.detail.history")}
        badge={reservation.history.length}
      >
        <Timeline
          items={[...reservation.history].reverse().map((entry, index) => ({
            id: `${index}-${entry.type}`,
            title: t(EVENT_LABEL_KEYS[entry.type] ?? entry.type),
            description: entry.to_status
              ? statusLabel(t, entry.to_status)
              : undefined,
            meta: when(entry.at),
          }))}
        />
      </EntitySectionCard>
    </>
  );
}

/** Deadlines while allowed; the reason once the API says no. */
function ActionNotes({
  reservation,
  when,
}: {
  reservation: CustomerReservation;
  when: (iso: string) => string;
}) {
  const { t } = useLocale();
  const open =
    reservation.status === "requested" || reservation.status === "active";
  if (!open) return null;

  const line = (action: CustomerAction, allowedKey: string, prefix: string) => {
    if (action.allowed) {
      return action.until ? t(allowedKey, { at: when(action.until) }) : null;
    }
    const reason = action.reason ? REASON_KEYS[action.reason] : undefined;
    return reason ? `${prefix}: ${t(reason)}` : null;
  };
  const notes = [
    line(
      reservation.actions.cancel,
      "customer_panel.detail.cancel_until",
      t("customer_panel.actions.cancel"),
    ),
    line(
      reservation.actions.reschedule,
      "customer_panel.detail.reschedule_until",
      t("customer_panel.actions.reschedule"),
    ),
  ].filter((note): note is string => Boolean(note));
  if (!notes.length) return null;

  return (
    <Alert>
      <CalendarClock className="size-4" />
      <AlertDescription>
        <ul className="space-y-1">
          {notes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

function LinesCard({ reservation }: { reservation: CustomerReservation }) {
  const { t, locale } = useLocale();
  const zone = reservation.timezone;
  const data = useMemo<Line[]>(
    () => reservation.lines.map((line, index) => ({ ...line, index })),
    [reservation.lines],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Line>({
          accessorKey: "starts_at",
          labelKey: "customer_panel.lines.time",
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
        createColumn<Line>({
          accessorKey: "service_name",
          labelKey: "customer_panel.lines.service",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.service_name}</span>
          ),
        }),
        createColumn<Line>({
          id: "staff",
          accessorFn: (row) => row.staff_name ?? "",
          labelKey: "customer_panel.lines.staff",
          enableSorting: false,
          cell: ({ row }) => row.original.staff_name ?? "—",
        }),
        createColumn<Line>({
          accessorKey: "duration_minutes",
          labelKey: "customer_panel.lines.duration",
          enableSorting: false,
          cell: ({ row }) =>
            t("customer_panel.lines.minutes", {
              minutes: row.original.duration_minutes,
            }),
        }),
        createColumn<Line>({
          accessorKey: "unit_price",
          labelKey: "customer_panel.lines.price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(
                row.original.unit_price,
                reservation.money.currency,
                locale,
              )}
            </span>
          ),
        }),
      ] as ColumnDef<Line, unknown>[],
    [locale, reservation.money.currency, t, zone],
  );

  return (
    <EntitySectionCard
      title={t("customer_panel.detail.lines")}
      badge={reservation.lines.length}
    >
      <EntityTable
        columns={columns}
        data={data}
        getRowId={(row) => String(row.index)}
        emptyTitle={t("customer_panel.lines.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "customer-panel-lines-v1",
        }}
      />
    </EntitySectionCard>
  );
}

type CustomerRefund = CustomerReservation["refunds"][number];

const REFUND_STATUS = {
  requested: {
    key: "customer_panel.refunds.status.requested",
    tone: "warning",
  },
  succeeded: {
    key: "customer_panel.refunds.status.succeeded",
    tone: "success",
  },
  failed: { key: "customer_panel.refunds.status.failed", tone: "danger" },
} as const;

/** Refunds of the visit: under way, paid back, or followed up by the business. */
function RefundsCard({ reservation }: { reservation: CustomerReservation }) {
  const { t, locale } = useLocale();
  const zone = reservation.timezone;
  const columns = useMemo(
    () =>
      [
        createColumn<CustomerRefund>({
          accessorKey: "requested_at",
          labelKey: "customer_panel.refunds.columns.requested_at",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetimeInZone(row.original.requested_at, zone, locale, {
                dateStyle: "medium",
                timeStyle: "short",
              })}
            </span>
          ),
        }),
        createColumn<CustomerRefund>({
          accessorKey: "amount",
          labelKey: "customer_panel.refunds.columns.amount",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {amount(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
        createColumn<CustomerRefund>({
          accessorKey: "status",
          labelKey: "customer_panel.refunds.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(REFUND_STATUS[row.original.status].key)}
              tone={REFUND_STATUS[row.original.status].tone}
            />
          ),
        }),
        createColumn<CustomerRefund>({
          id: "refunded_at",
          accessorFn: (row) => row.refunded_at ?? "",
          labelKey: "customer_panel.refunds.columns.refunded_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.refunded_at
              ? datetimeInZone(row.original.refunded_at, zone, locale, {
                  dateStyle: "medium",
                  timeStyle: "short",
                })
              : "—",
        }),
      ] as ColumnDef<CustomerRefund, unknown>[],
    [locale, t, zone],
  );

  if (!reservation.refunds.length) return null;
  const open = reservation.refunds.some((r) => r.status !== "succeeded");

  return (
    <EntitySectionCard
      title={t("customer_panel.refunds.title")}
      badge={reservation.refunds.length}
    >
      <div className="space-y-3">
        {open ? (
          <Alert>
            <AlertDescription>
              {t("customer_panel.refunds.hint")}
            </AlertDescription>
          </Alert>
        ) : null}
        <EntityTable
          columns={columns}
          data={reservation.refunds}
          getRowId={(row) => row.uuid}
          emptyTitle={t("customer_panel.refunds.empty")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
            globalFilter: false,
            sorting: false,
            pagination: false,
            persistKey: "customer-panel-refunds-v1",
          }}
        />
      </div>
    </EntitySectionCard>
  );
}

function PoliciesCard({ reservation }: { reservation: CustomerReservation }) {
  const { t } = useLocale();
  const { policies } = reservation;
  const fields = [
    {
      key: "cancellation",
      label: t("customer_panel.policies.cancellation"),
      value: policies.cancellation,
    },
    {
      key: "change",
      label: t("customer_panel.policies.change"),
      value: policies.change,
    },
    {
      key: "no_show",
      label: t("customer_panel.policies.no_show"),
      value: policies.no_show,
    },
  ].filter((field) => field.value.trim());
  if (!fields.length) return null;

  return (
    <EntitySectionCard title={t("customer_panel.policies.title")}>
      <EntityDetail
        sections={[
          {
            id: "policies",
            fields: fields.map((field) => ({
              ...field,
              value: <span className="whitespace-pre-line">{field.value}</span>,
            })),
          },
        ]}
      />
    </EntitySectionCard>
  );
}

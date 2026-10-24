"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  INTEGRATION_PROVIDER_LABEL_KEYS,
  labelOf,
} from "@/features/integrations/constants";
import {
  useCloseWithoutRefund,
  useRefundReservationPayment,
  useResolveRefund,
} from "@/features/reservations/hooks/use-reservations";
import {
  createRefundSchema,
  MAX_REFUND_NOTE,
  parseAmount,
  REFUND_CHANNELS,
  REFUND_METHODS,
  type RefundFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import type {
  Refund,
  RefundCollection,
  ReservationPayments,
} from "@/features/reservations/types";
import { amount, date, datetime, uuid } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const REFUND_FORM_ID = "reservation-refund-form";
const RESOLVE_FORM_ID = "reservation-refund-resolve-form";
const CLOSE_FORM_ID = "reservation-refund-close-form";

const STATUS_TONES = {
  requested: "warning",
  succeeded: "success",
  failed: "danger",
} as const;

/** Status label of a refund: an open one says why it is still open. */
function statusKey(refund: Refund) {
  if (refund.status !== "requested") {
    return `reservations.refunds.status.${refund.status}`;
  }
  return refund.provider_status === "succeeded"
    ? "reservations.refunds.status.confirmed"
    : refund.provider_status === "uncertain"
      ? "reservations.refunds.status.uncertain"
      : "reservations.refunds.status.requested";
}

/** A new key per opened dialog: a double click or a retry is one refund. */
function newIdempotencyKey() {
  return `ui:${uuid()}`;
}

/** Refunds of a visit; open ones a manager can settle. */
export function RefundsTable({
  view,
  canResolve,
  onResolve,
}: {
  view: ReservationPayments;
  canResolve: boolean;
  onResolve: (refund: Refund) => void;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<Refund>({
          accessorKey: "created_at",
          labelKey: "reservations.refunds.columns.date",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<Refund>({
          accessorKey: "channel",
          labelKey: "reservations.refunds.columns.channel",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">
                {row.original.channel === "provider"
                  ? labelOf(
                      t,
                      INTEGRATION_PROVIDER_LABEL_KEYS,
                      row.original.provider,
                    )
                  : t(`reservations.payments.methods.${row.original.method}`)}
              </span>
              <span className="text-muted-foreground text-xs">
                {t(`reservations.refunds.channels.${row.original.channel}`)}
              </span>
            </span>
          ),
        }),
        createColumn<Refund>({
          accessorKey: "status",
          labelKey: "reservations.refunds.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col gap-1">
              <StatusChip
                label={t(statusKey(row.original))}
                tone={STATUS_TONES[row.original.status]}
              />
              {row.original.failure_reason ? (
                <span className="text-muted-foreground text-xs">
                  {row.original.failure_reason}
                </span>
              ) : null}
              {row.original.auto_check ? (
                <span className="text-muted-foreground text-xs">
                  {row.original.next_check_at
                    ? t("reservations.refunds.auto_check_next", {
                        time: datetime(
                          row.original.next_check_at,
                          undefined,
                          locale,
                        ),
                      })
                    : t("reservations.refunds.auto_check")}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<Refund>({
          accessorKey: "reason",
          labelKey: "reservations.refunds.columns.reason",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reason || row.original.resolution_note || "—",
        }),
        createColumn<Refund>({
          id: "by",
          accessorFn: (row) => row.requested_by?.name ?? "",
          labelKey: "reservations.payments.columns.by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.requested_by?.name ?? "—",
        }),
        createColumn<Refund>({
          accessorKey: "amount",
          labelKey: "reservations.payments.columns.amount",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              −{amount(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
        createColumn<Refund>({
          id: "actions",
          labelKey: "reservations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canResolve && row.original.needs_action ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={(event) => {
                  event.stopPropagation();
                  onResolve(row.original);
                }}
              >
                <CircleCheck className="size-4" />
                {t("reservations.refunds.resolve")}
              </Button>
            ) : null,
        }),
      ] as ColumnDef<Refund, unknown>[],
    [canResolve, locale, onResolve, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={view.refunds}
      getRowId={(row) => row.uuid}
      emptyTitle={t("reservations.refunds.empty")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
        globalFilter: false,
        sorting: false,
        pagination: false,
        persistKey: "tenant-reservation-refunds-v1",
      }}
    />
  );
}

/** Keeps channel, amount and account in step with the chosen collection. */
function RefundFields({
  view,
  collections,
}: {
  view: ReservationPayments;
  collections: RefundCollection[];
}) {
  const { t, locale } = useLocale();
  const { setValue, control } = useFormContext<RefundFormValues>();
  const [collectionUUID, channel] = useWatch({
    control,
    name: ["collection_uuid", "channel"],
  });
  const collection = collections.find((c) => c.uuid === collectionUUID);
  const previous = useRef(collectionUUID);

  useEffect(() => {
    if (previous.current === collectionUUID || !collection) return;
    previous.current = collectionUUID;
    setValue("amount", collection.refundable);
    setValue("channel", collection.provider_refund ? "provider" : "manual");
    setValue(
      "method",
      (REFUND_METHODS as readonly string[]).includes(collection.method)
        ? (collection.method as RefundFormValues["method"])
        : "cash",
    );
  }, [collection, collectionUUID, setValue]);

  const collectionOptions = collections.map((c) => ({
    value: c.uuid,
    label: t("reservations.refunds.fields.collection_option", {
      date: date(c.entry_date, undefined, locale),
      method: t(`reservations.payments.methods.${c.method}`),
      amount: amount(c.amount, view.currency, locale),
      refundable: amount(c.refundable, view.currency, locale),
    }),
  }));
  const accountOptions = view.refund_accounts.map((account) => ({
    value: account.uuid,
    label: account.branch
      ? t("reservations.payments.fields.account_branch", {
          name: account.name,
        })
      : account.name,
  }));

  return (
    <>
      <AppSelect
        name="collection_uuid"
        label={t("reservations.refunds.fields.collection")}
        placeholder={t("reservations.refunds.fields.collection_placeholder")}
        options={collectionOptions}
      />
      <AppInput
        name="amount"
        inputMode="decimal"
        label={t("reservations.payments.fields.amount", {
          currency: view.currency,
        })}
        description={
          collection
            ? t("reservations.refunds.fields.amount_hint", {
                refundable: amount(
                  collection.refundable,
                  view.currency,
                  locale,
                ),
              })
            : undefined
        }
      />
      {collection?.provider_refund ? (
        <AppToggleGroup
          name="channel"
          label={t("reservations.refunds.fields.channel")}
          options={REFUND_CHANNELS.map((value) => ({
            value,
            label:
              value === "provider"
                ? t("reservations.refunds.fields.via_provider", {
                    provider: labelOf(
                      t,
                      INTEGRATION_PROVIDER_LABEL_KEYS,
                      collection.provider,
                    ),
                  })
                : t("reservations.refunds.channels.manual"),
          }))}
        />
      ) : null}
      {channel === "manual" ? (
        <>
          <AppToggleGroup
            name="method"
            label={t("reservations.payments.fields.method")}
            options={REFUND_METHODS.map((method) => ({
              value: method,
              label: t(`reservations.payments.methods.${method}`),
            }))}
          />
          {accountOptions.length === 0 ? (
            <Alert>
              <AlertDescription>
                {t("reservations.refunds.fields.no_accounts")}
              </AlertDescription>
            </Alert>
          ) : (
            <AppSelect
              name="cash_account_uuid"
              label={t("reservations.refunds.fields.account")}
              description={t("reservations.refunds.fields.account_hint")}
              placeholder={t(
                "reservations.payments.fields.account_placeholder",
              )}
              options={accountOptions}
            />
          )}
        </>
      ) : (
        <Alert>
          <AlertDescription>
            {t("reservations.refunds.fields.provider_hint")}
          </AlertDescription>
        </Alert>
      )}
      <AppTextarea
        name="reason"
        label={t("reservations.refunds.fields.reason")}
      />
    </>
  );
}

/**
 * Pays money back on one collection: through the provider when it can
 * refund the online payment, otherwise recorded by hand from a till or an
 * account. Defaults to everything left of the newest collection.
 */
export function RefundDialog({
  uuid,
  view,
  open,
  onOpenChange,
}: {
  uuid: string;
  view: ReservationPayments | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const refund = useRefundReservationPayment(uuid);
  const [key, setKey] = useState(newIdempotencyKey);
  const collections = useMemo(
    () =>
      (view?.refund_collections ?? []).filter(
        (c) => parseAmount(c.refundable) > 0,
      ),
    [view?.refund_collections],
  );
  const schema = useMemo(
    () =>
      createRefundSchema(t, (id) =>
        parseAmount(collections.find((c) => c.uuid === id)?.refundable ?? "0"),
      ),
    [collections, t],
  );

  const change = (next: boolean) => {
    if (next) setKey(newIdempotencyKey());
    onOpenChange(next);
  };

  const first = collections[0];
  const accounts = view?.refund_accounts ?? [];
  const defaultAccount =
    accounts.find((a) => a.uuid === first?.cash_account.uuid)?.uuid ??
    accounts[0]?.uuid ??
    "";

  return (
    <AppDialog
      open={open}
      onOpenChange={change}
      title={t("reservations.refunds.dialog.title")}
      description={
        view
          ? t("reservations.refunds.dialog.description", {
              number: view.reservation.number,
              refundable: amount(view.refundable, view.currency, locale),
            })
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => change(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={REFUND_FORM_ID}
            disabled={refund.isPending || collections.length === 0}
          >
            {t("reservations.refunds.dialog.submit")}
          </Button>
        </>
      }
    >
      {open && view ? (
        collections.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("reservations.refunds.dialog.nothing")}
            </AlertDescription>
          </Alert>
        ) : (
          <AppForm<RefundFormValues>
            key={key}
            id={REFUND_FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              collection_uuid: first?.uuid ?? "",
              amount: first?.refundable ?? "",
              channel: first?.provider_refund ? "provider" : "manual",
              method: (REFUND_METHODS as readonly string[]).includes(
                first?.method ?? "",
              )
                ? (first?.method as RefundFormValues["method"])
                : "cash",
              cash_account_uuid: defaultAccount,
              reason: "",
            }}
            onSubmit={async (values) => {
              const manual = values.channel === "manual";
              await refund.mutateAsync({
                collection_uuid: values.collection_uuid,
                amount: parseAmount(values.amount).toFixed(2),
                channel: values.channel,
                ...(manual
                  ? {
                      method: values.method,
                      cash_account_uuid: values.cash_account_uuid,
                    }
                  : {}),
                ...(values.reason.trim()
                  ? { reason: values.reason.trim() }
                  : {}),
                idempotency_key: key,
              });
              change(false);
            }}
          >
            <RefundFields view={view} collections={collections} />
          </AppForm>
        )
      ) : null}
    </AppDialog>
  );
}

const RESOLVE_OUTCOMES = ["succeeded", "failed"] as const;

/**
 * Settles an open provider refund after someone checked the provider's
 * panel: paid back (posted now) or not paid (ended). One the provider
 * confirmed can only be posted.
 */
export function ResolveRefundDialog({
  reservationUUID,
  refund,
  onOpenChange,
}: {
  reservationUUID: string;
  refund: Refund | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const resolve = useResolveRefund(reservationUUID);
  const confirmed = refund?.provider_status === "succeeded";
  const schema = useMemo(
    () =>
      z.object({
        outcome: z.enum(RESOLVE_OUTCOMES),
        note: z
          .string()
          .max(MAX_REFUND_NOTE, t("reservations.validation.collect_note_max")),
      }),
    [t],
  );
  const outcomes = confirmed ? (["succeeded"] as const) : RESOLVE_OUTCOMES;

  return (
    <AppDialog
      open={Boolean(refund)}
      onOpenChange={onOpenChange}
      title={t("reservations.refunds.resolve_dialog.title")}
      description={
        refund
          ? t(
              confirmed
                ? "reservations.refunds.resolve_dialog.confirmed"
                : "reservations.refunds.resolve_dialog.description",
              {
                amount: amount(refund.amount, refund.currency, locale),
                provider: labelOf(
                  t,
                  INTEGRATION_PROVIDER_LABEL_KEYS,
                  refund.provider,
                ),
              },
            )
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={RESOLVE_FORM_ID}
            disabled={resolve.isPending}
          >
            {t("reservations.refunds.resolve_dialog.submit")}
          </Button>
        </>
      }
    >
      {refund ? (
        <AppForm<{ outcome: (typeof RESOLVE_OUTCOMES)[number]; note: string }>
          key={refund.uuid}
          id={RESOLVE_FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ outcome: "succeeded", note: "" }}
          onSubmit={async (values) => {
            await resolve.mutateAsync({
              uuid: refund.uuid,
              body: {
                outcome: values.outcome,
                ...(values.note.trim() ? { note: values.note.trim() } : {}),
              },
            });
            onOpenChange(false);
          }}
        >
          <AppToggleGroup
            name="outcome"
            label={t("reservations.refunds.resolve_dialog.outcome")}
            options={outcomes.map((value) => ({
              value,
              label: t(`reservations.refunds.resolve_dialog.outcomes.${value}`),
            }))}
          />
          <AppTextarea
            name="note"
            label={t("reservations.refunds.resolve_dialog.note")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** The visit whose refund to make is closed without paying it back. */
export type CloseTarget = {
  uuid: string;
  number: number;
  amount: string;
  currency: string;
};

/**
 * Closes a refund to make without paying the money back (the customer left
 * it, or it counts for the next visit). The reason is required and goes
 * into the visit's history; no money moves.
 */
export function CloseWithoutRefundDialog({
  target,
  onOpenChange,
}: {
  target: CloseTarget | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const close = useCloseWithoutRefund();
  const schema = useMemo(
    () =>
      z.object({
        reason: z
          .string()
          .trim()
          .min(1, t("reservations.validation.close_reason_required"))
          .max(MAX_REFUND_NOTE, t("reservations.validation.collect_note_max")),
      }),
    [t],
  );

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={onOpenChange}
      title={t("reservations.refunds.close_dialog.title")}
      description={
        target
          ? t("reservations.refunds.close_dialog.description", {
              number: target.number,
              amount: amount(target.amount, target.currency, locale),
            })
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={CLOSE_FORM_ID} disabled={close.isPending}>
            {t("reservations.refunds.close_dialog.submit")}
          </Button>
        </>
      }
    >
      {target ? (
        <AppForm<{ reason: string }>
          key={target.uuid}
          id={CLOSE_FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ reason: "" }}
          onSubmit={async (values) => {
            await close.mutateAsync({
              uuid: target.uuid,
              body: { reason: values.reason.trim() },
            });
            onOpenChange(false);
          }}
        >
          <Alert>
            <AlertDescription>
              {t("reservations.refunds.close_dialog.hint")}
            </AlertDescription>
          </Alert>
          <AppTextarea
            name="reason"
            label={t("reservations.refunds.close_dialog.reason")}
            placeholder={t(
              "reservations.refunds.close_dialog.reason_placeholder",
            )}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

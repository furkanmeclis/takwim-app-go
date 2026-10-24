"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Archive, HandCoins, Undo2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDetail,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
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
import { permissions } from "@/config/permissions";
import {
  INTEGRATION_PROVIDER_LABEL_KEYS,
  labelOf,
} from "@/features/integrations/constants";
import {
  type CloseTarget,
  CloseWithoutRefundDialog,
  RefundDialog,
  RefundsTable,
  ResolveRefundDialog,
} from "@/features/reservations/components/reservation-refunds";
import {
  useCollectReservationPayment,
  useReservationPayments,
} from "@/features/reservations/hooks/use-reservations";
import {
  COLLECT_METHODS,
  createCollectSchema,
  parseAmount,
  type CollectFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import type {
  PaymentAttempt,
  Refund,
  ReservationLedgerEntry,
  ReservationPayments,
} from "@/features/reservations/types";
import { amount, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "reservation-collect-form";

const ATTEMPT_TONES = {
  pending: "warning",
  succeeded: "success",
  failed: "danger",
} as const;

/** Money in (+) or out (−) of the customer for one entry. */
function signOf(entry: ReservationLedgerEntry) {
  const out = entry.effective_kind === "refund";
  return entry.kind === "reversal" ? (out ? "+" : "−") : out ? "−" : "+";
}

/**
 * Money of a visit: what is expected, paid and left, the ledger entries
 * (collections, refunds, reversals) and the online payment attempts; the
 * "take payment" dialog records an on-site payment (cash, card, transfer).
 */
export function ReservationPaymentsCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const query = useReservationPayments(uuid);
  const [collecting, setCollecting] = useState(false);
  const [refunding, setRefunding] = useState(false);
  const [resolving, setResolving] = useState<Refund | null>(null);
  const [closing, setClosing] = useState<CloseTarget | null>(null);
  const view = query.data;
  const money = (value: string | null | undefined) =>
    amount(value ?? "0", view?.currency ?? "TRY", locale);

  const entryColumns = useMemo(
    () =>
      [
        createColumn<ReservationLedgerEntry>({
          accessorKey: "created_at",
          labelKey: "reservations.payments.columns.date",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<ReservationLedgerEntry>({
          accessorKey: "kind",
          labelKey: "reservations.payments.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {t(`reservations.payments.kinds.${row.original.kind}`)}
            </span>
          ),
        }),
        createColumn<ReservationLedgerEntry>({
          accessorKey: "method",
          labelKey: "reservations.payments.columns.method",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.method
              ? t(`reservations.payments.methods.${row.original.method}`)
              : "—",
        }),
        createColumn<ReservationLedgerEntry>({
          id: "account",
          accessorFn: (row) => row.cash_account?.name ?? "",
          labelKey: "reservations.payments.columns.account",
          enableSorting: false,
          cell: ({ row }) => row.original.cash_account?.name ?? "—",
        }),
        createColumn<ReservationLedgerEntry>({
          id: "by",
          accessorFn: (row) => row.created_by?.name ?? "",
          labelKey: "reservations.payments.columns.by",
          enableSorting: false,
          cell: ({ row }) => row.original.created_by?.name ?? "—",
        }),
        createColumn<ReservationLedgerEntry>({
          accessorKey: "amount",
          labelKey: "reservations.payments.columns.amount",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {signOf(row.original)}
              {amount(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<ReservationLedgerEntry, unknown>[],
    [locale, t],
  );

  const attemptColumns = useMemo(
    () =>
      [
        createColumn<PaymentAttempt>({
          accessorKey: "created_at",
          labelKey: "reservations.payments.columns.started",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<PaymentAttempt>({
          accessorKey: "provider",
          labelKey: "reservations.payments.columns.provider",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {labelOf(
                t,
                INTEGRATION_PROVIDER_LABEL_KEYS,
                row.original.provider,
              )}
            </span>
          ),
        }),
        createColumn<PaymentAttempt>({
          accessorKey: "status",
          labelKey: "reservations.payments.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`reservations.payments.attempt.${row.original.status}`)}
              tone={ATTEMPT_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<PaymentAttempt>({
          accessorKey: "outcome",
          labelKey: "reservations.payments.columns.outcome",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.outcome
              ? t(`reservations.payments.outcome.${row.original.outcome}`)
              : row.original.failure_reason || "—",
        }),
        createColumn<PaymentAttempt>({
          accessorKey: "amount",
          labelKey: "reservations.payments.columns.amount",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<PaymentAttempt, unknown>[],
    [locale, t],
  );

  const canCollect =
    Boolean(view?.can_collect) &&
    can(permissions.tenant.payment.collect) &&
    parseAmount(view?.remaining ?? "0") > 0;
  const canRefundPermission = can(permissions.tenant.payment.refund);
  const canRefund =
    Boolean(view?.can_refund) &&
    canRefundPermission &&
    parseAmount(view?.refundable ?? "0") > 0;
  const canClose = Boolean(view?.can_close) && canRefundPermission;

  return (
    <EntitySectionCard
      title={t("reservations.detail.payments")}
      actions={
        canCollect || canRefund || canClose ? (
          <div className="flex flex-wrap gap-2">
            {canCollect ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setCollecting(true)}
              >
                <HandCoins className="size-4" />
                {t("reservations.payments.collect")}
              </Button>
            ) : null}
            {canRefund ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setRefunding(true)}
              >
                <Undo2 className="size-4" />
                {t("reservations.refunds.action")}
              </Button>
            ) : null}
            {canClose && view ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() =>
                  setClosing({
                    uuid,
                    number: view.reservation.number,
                    amount: view.refundable,
                    currency: view.currency,
                  })
                }
              >
                <Archive className="size-4" />
                {t("reservations.refunds.close")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("reservations.payments.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {view ? (
        <div className="space-y-6">
          {view.kept ? (
            <Alert>
              <AlertDescription>
                {t(
                  view.kept.by
                    ? "reservations.refunds.kept_by"
                    : "reservations.refunds.kept",
                  {
                    date: datetime(view.kept.at, undefined, locale),
                    name: view.kept.by?.name ?? "",
                    reason: view.kept.reason,
                  },
                )}
              </AlertDescription>
            </Alert>
          ) : null}
          <EntityDetail
            sections={[
              {
                id: "money",
                fields: [
                  {
                    key: "expected",
                    label: t("reservations.payments.expected"),
                    value: money(view.expected),
                  },
                  {
                    key: "paid",
                    label: t("reservations.payments.paid"),
                    value: money(view.paid),
                  },
                  {
                    key: "remaining",
                    label: t("reservations.payments.remaining"),
                    value: money(view.remaining),
                  },
                  ...(parseAmount(view.credit) > 0
                    ? [
                        {
                          key: "credit",
                          label: t("reservations.payments.credit"),
                          value: money(view.credit),
                        },
                      ]
                    : []),
                  ...(parseAmount(view.refundable) > 0
                    ? [
                        {
                          key: "refundable",
                          label: t("reservations.refunds.refundable"),
                          value: money(view.refundable),
                        },
                      ]
                    : []),
                ],
              },
            ]}
          />
          <div className="space-y-2">
            <h3 className="text-sm font-medium">
              {t("reservations.payments.entries")}
            </h3>
            <EntityTable
              columns={entryColumns}
              data={view.entries}
              getRowId={(row) => row.uuid}
              emptyTitle={t("reservations.payments.entries_empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
                globalFilter: false,
                sorting: false,
                pagination: false,
                persistKey: "tenant-reservation-payments-v1",
              }}
            />
          </div>
          {view.online.length > 0 ? (
            <div className="space-y-2">
              <h3 className="text-sm font-medium">
                {t("reservations.payments.online")}
              </h3>
              <EntityTable
                columns={attemptColumns}
                data={view.online}
                getRowId={(row) => row.uuid}
                emptyTitle={t("reservations.payments.online_empty")}
                manual={{
                  filtering: false,
                  sorting: false,
                  pagination: false,
                }}
                features={{
                  rowSelection: false,
                  columnFilters: false,
                  facetedFilters: false,
                  globalFilter: false,
                  sorting: false,
                  pagination: false,
                  persistKey: "tenant-reservation-payment-attempts-v1",
                }}
              />
            </div>
          ) : null}
          {view.refunds.length > 0 ? (
            <div className="space-y-2">
              <h3 className="text-sm font-medium">
                {t("reservations.refunds.title")}
              </h3>
              {view.refunds.some((refund) => refund.needs_action) ? (
                <Alert>
                  <AlertDescription>
                    {t("reservations.refunds.needs_action")}
                  </AlertDescription>
                </Alert>
              ) : null}
              <RefundsTable
                view={view}
                canResolve={canRefundPermission}
                onResolve={setResolving}
              />
            </div>
          ) : null}
        </div>
      ) : null}

      <CollectDialog
        uuid={uuid}
        view={view}
        open={collecting}
        onOpenChange={setCollecting}
      />
      <RefundDialog
        uuid={uuid}
        view={view}
        open={refunding}
        onOpenChange={setRefunding}
      />
      <CloseWithoutRefundDialog
        target={closing}
        onOpenChange={(open) => {
          if (!open) setClosing(null);
        }}
      />
      <ResolveRefundDialog
        reservationUUID={uuid}
        refund={resolving}
        onOpenChange={(open) => {
          if (!open) setResolving(null);
        }}
      />
    </EntitySectionCard>
  );
}

function CollectDialog({
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
  const collect = useCollectReservationPayment(uuid);
  const remaining = parseAmount(view?.remaining ?? "0");
  const schema = useMemo(
    () => createCollectSchema(t, remaining),
    [t, remaining],
  );
  const accounts = view?.collect_accounts ?? [];
  const accountOptions = accounts.map((account) => ({
    value: account.uuid,
    label: account.branch
      ? t("reservations.payments.fields.account_branch", {
          name: account.name,
        })
      : account.name,
  }));

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("reservations.payments.dialog.title")}
      description={
        view
          ? t("reservations.payments.dialog.description", {
              number: view.reservation.number,
              remaining: amount(view.remaining, view.currency, locale),
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
          <Button
            type="submit"
            form={FORM_ID}
            disabled={collect.isPending || accounts.length === 0}
          >
            {t("reservations.payments.dialog.submit")}
          </Button>
        </>
      }
    >
      {open && view ? (
        accounts.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("reservations.payments.fields.no_accounts")}
            </AlertDescription>
          </Alert>
        ) : (
          <AppForm<CollectFormValues>
            key={`${uuid}-${view.remaining}`}
            id={FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              amount: view.remaining,
              method: "cash",
              cash_account_uuid: accounts[0]?.uuid ?? "",
              note: "",
            }}
            onSubmit={async (values) => {
              await collect.mutateAsync({
                amount: parseAmount(values.amount).toFixed(2),
                method: values.method,
                cash_account_uuid: values.cash_account_uuid,
                ...(values.note.trim() ? { note: values.note.trim() } : {}),
              });
              onOpenChange(false);
            }}
          >
            <AppInput
              name="amount"
              inputMode="decimal"
              label={t("reservations.payments.fields.amount", {
                currency: view.currency,
              })}
              description={t("reservations.payments.fields.amount_hint")}
            />
            <AppToggleGroup
              name="method"
              label={t("reservations.payments.fields.method")}
              options={COLLECT_METHODS.map((method) => ({
                value: method,
                label: t(`reservations.payments.methods.${method}`),
              }))}
            />
            <AppSelect
              name="cash_account_uuid"
              label={t("reservations.payments.fields.account")}
              placeholder={t(
                "reservations.payments.fields.account_placeholder",
              )}
              options={accountOptions}
            />
            <AppTextarea
              name="note"
              label={t("reservations.payments.fields.note")}
            />
          </AppForm>
        )
      ) : null}
    </AppDialog>
  );
}

"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  useCollectStockSale,
  useRefundStockSale,
  useStockSalePayments,
} from "@/features/products/hooks/use-products";
import {
  accountOptions,
  localizeProductError,
} from "@/features/products/lib/format";
import {
  COLLECT_METHODS,
  MAX_COLLECT_NOTE,
  REFUND_METHODS,
} from "@/features/products/schemas/movement-form";
import type { StockMovement } from "@/features/products/types";
import { amount as formatAmount } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const COLLECT_FORM_ID = "stock-sale-collect-form";
const REFUND_FORM_ID = "stock-sale-refund-form";
const AMOUNT = /^\d+([.,]\d{1,2})?$/;

/** "450,5" → 450.5 (NaN when not an amount). */
function parseAmount(value: string) {
  return AMOUNT.test(value.trim())
    ? Number(value.trim().replace(",", "."))
    : NaN;
}

/** A new key per opened dialog: a double click or a retry is one refund. */
function newIdempotencyKey() {
  return `ui:${crypto.randomUUID()}`;
}

function saleLabel(movement: StockMovement) {
  return movement.variant
    ? `${movement.product.name} · ${movement.variant.name}`
    : movement.product.name;
}

type CollectFormValues = {
  amount: string;
  method: (typeof COLLECT_METHODS)[number];
  cash_account_uuid: string;
  note: string;
};

/**
 * Takes money for a counter sale recorded without it, or the rest of one
 * paid in parts (TEC-992): at most what is left to pay, into a till or
 * bank account of the branch.
 */
export function StockSaleCollectDialog({
  movement,
  onOpenChange,
}: {
  movement: StockMovement | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const collect = useCollectStockSale();
  const paymentsQuery = useStockSalePayments(movement?.uuid ?? null);
  const view = paymentsQuery.data;
  const remaining = parseAmount(view?.remaining ?? "0") || 0;
  const accounts = view?.collect_accounts ?? [];
  const schema = useMemo(
    () =>
      z.object({
        amount: z
          .string()
          .trim()
          .refine((v) => parseAmount(v) > 0, t("products.validation.amount"))
          .refine(
            (v) => !(parseAmount(v) > remaining + 0.0001),
            t("products.validation.amount_over_remaining"),
          ),
        method: z.enum(COLLECT_METHODS),
        cash_account_uuid: z
          .string()
          .min(1, t("products.validation.account_required")),
        note: z
          .string()
          .max(MAX_COLLECT_NOTE, t("products.validation.note_max")),
      }),
    [remaining, t],
  );

  return (
    <AppDialog
      open={Boolean(movement)}
      onOpenChange={onOpenChange}
      title={t("products.movements.collect.title")}
      description={
        movement
          ? t("products.movements.collect.description", {
              product: saleLabel(movement),
              remaining: formatAmount(view?.remaining, view?.currency, locale),
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
            form={COLLECT_FORM_ID}
            disabled={collect.isPending || !view || accounts.length === 0}
          >
            {t("products.movements.collect.submit")}
          </Button>
        </>
      }
    >
      {movement && view ? (
        accounts.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("products.movements.collection.no_accounts")}
            </AlertDescription>
          </Alert>
        ) : (
          <AppForm<CollectFormValues>
            key={`${movement.uuid}-${view.remaining}`}
            id={COLLECT_FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              amount: view.remaining,
              method: "cash",
              cash_account_uuid: accounts[0]?.uuid ?? "",
              note: "",
            }}
            onSubmit={async (values) => {
              try {
                await collect.mutateAsync({
                  uuid: movement.uuid,
                  body: {
                    amount: parseAmount(values.amount).toFixed(2),
                    method: values.method,
                    cash_account_uuid: values.cash_account_uuid,
                    ...(values.note.trim() ? { note: values.note.trim() } : {}),
                  },
                });
              } catch (error) {
                localizeProductError(error, t);
                throw error;
              }
              onOpenChange(false);
            }}
          >
            <AppInput
              name="amount"
              inputMode="decimal"
              label={t("products.movements.collection.amount")}
              description={t("products.movements.collect.amount_hint")}
            />
            <AppToggleGroup
              name="method"
              label={t("products.movements.collection.method")}
              options={COLLECT_METHODS.map((method) => ({
                value: method,
                label: t(`products.movements.methods.${method}`),
              }))}
            />
            <AppSelect
              name="cash_account_uuid"
              label={t("products.movements.collection.account")}
              placeholder={t(
                "products.movements.collection.account_placeholder",
              )}
              options={accountOptions(accounts, t)}
            />
            <AppTextarea
              name="note"
              label={t("products.movements.collection.note")}
            />
          </AppForm>
        )
      ) : null}
    </AppDialog>
  );
}

type RefundFormValues = {
  amount: string;
  method: (typeof REFUND_METHODS)[number];
  cash_account_uuid: string;
  reason: string;
};

/**
 * Pays the money of a sale back after a return made without it (TEC-992):
 * at most what was collected, by hand, out of a till, bank or online
 * account of the branch. One idempotency key per opened dialog.
 */
export function StockSaleRefundDialog({
  movement,
  onOpenChange,
}: {
  movement: StockMovement | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const refund = useRefundStockSale();
  const [key, setKey] = useState(newIdempotencyKey);
  const paymentsQuery = useStockSalePayments(movement?.uuid ?? null);
  const view = paymentsQuery.data;
  const refundable = parseAmount(view?.refundable ?? "0") || 0;
  const accounts = view?.refund_accounts ?? [];
  const schema = useMemo(
    () =>
      z.object({
        amount: z
          .string()
          .trim()
          .refine((v) => parseAmount(v) > 0, t("products.validation.amount"))
          .refine(
            (v) => !(parseAmount(v) > refundable + 0.0001),
            t("products.validation.amount_over_refundable"),
          ),
        method: z.enum(REFUND_METHODS),
        cash_account_uuid: z
          .string()
          .min(1, t("products.validation.account_required")),
        reason: z
          .string()
          .max(MAX_COLLECT_NOTE, t("products.validation.note_max")),
      }),
    [refundable, t],
  );

  const change = (next: boolean) => {
    if (next) setKey(newIdempotencyKey());
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={Boolean(movement)}
      onOpenChange={change}
      title={t("products.movements.refund.title")}
      description={
        movement
          ? t("products.movements.refund.description", {
              product: saleLabel(movement),
              refundable: formatAmount(
                view?.refundable,
                view?.currency,
                locale,
              ),
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
            disabled={refund.isPending || !view || accounts.length === 0}
          >
            {t("products.movements.refund.submit")}
          </Button>
        </>
      }
    >
      {movement && view ? (
        accounts.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("products.movements.collection.no_accounts")}
            </AlertDescription>
          </Alert>
        ) : (
          <AppForm<RefundFormValues>
            key={key}
            id={REFUND_FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              amount: view.refundable,
              method: "cash",
              cash_account_uuid: accounts[0]?.uuid ?? "",
              reason: "",
            }}
            onSubmit={async (values) => {
              try {
                await refund.mutateAsync({
                  uuid: movement.uuid,
                  body: {
                    amount: parseAmount(values.amount).toFixed(2),
                    method: values.method,
                    cash_account_uuid: values.cash_account_uuid,
                    idempotency_key: key,
                    ...(values.reason.trim()
                      ? { reason: values.reason.trim() }
                      : {}),
                  },
                });
              } catch (error) {
                localizeProductError(error, t);
                throw error;
              }
              change(false);
            }}
          >
            <AppInput
              name="amount"
              inputMode="decimal"
              label={t("products.movements.collection.amount")}
              description={t("products.movements.return.refund_amount_hint")}
            />
            <AppToggleGroup
              name="method"
              label={t("products.movements.collection.method")}
              options={REFUND_METHODS.map((method) => ({
                value: method,
                label: t(`products.movements.methods.${method}`),
              }))}
            />
            <AppSelect
              name="cash_account_uuid"
              label={t("products.movements.return.refund_account")}
              placeholder={t(
                "products.movements.collection.account_placeholder",
              )}
              options={accountOptions(accounts, t)}
            />
            <AppTextarea
              name="reason"
              label={t("products.movements.refund.reason")}
            />
          </AppForm>
        )
      ) : null}
    </AppDialog>
  );
}

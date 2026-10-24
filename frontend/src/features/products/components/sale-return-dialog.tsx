"use client";

import { useEffect, useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { permissions } from "@/config/permissions";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import {
  useReturnSale,
  useStockSalePayments,
} from "@/features/products/hooks/use-products";
import {
  accountOptions,
  localizeProductError,
} from "@/features/products/lib/format";
import {
  optionalPriceField,
  normalizePrice,
} from "@/features/products/schemas/product-form";
import {
  REFUND_METHODS,
  type RefundMethod,
} from "@/features/products/schemas/movement-form";
import type {
  StockMovement,
  StockSalePayments,
} from "@/features/products/types";
import { amount as formatAmount } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "stock-sale-return-form";
const MAX_REASON = 200;
const MAX_NOTE = 2000;

type ReturnFormValues = {
  reason: string;
  note: string;
  refund: {
    enabled: boolean;
    amount: string;
    method: RefundMethod;
    cash_account_uuid: string;
  };
};

function parseAmount(value: string) {
  const v = value.trim().replace(",", ".");
  return v ? Number(v) : NaN;
}

/**
 * A manager takes back the product of a sale: every unit returns to the
 * branch stock and the sale's revenue is reversed. When money was taken
 * for the sale, the dialog pays it back in the same step (TEC-992): the
 * amount (at most what was collected), the way and the account.
 */
export function SaleReturnDialog({
  movement,
  onOpenChange,
}: {
  movement: StockMovement | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const returnSale = useReturnSale();
  const paymentsQuery = useStockSalePayments(movement?.uuid ?? null);
  const view = paymentsQuery.data;
  const refundable = parseAmount(view?.refundable ?? "0") || 0;
  const canRefund =
    Boolean(view?.can_refund) &&
    can(permissions.tenant.payment.refund) &&
    hasModule("cash") &&
    refundable > 0;
  const schema = useMemo(
    () =>
      z
        .object({
          reason: z
            .string()
            .trim()
            .min(1, t("products.movements.return.reason_required"))
            .max(MAX_REASON, t("products.movements.return.reason_max")),
          note: z
            .string()
            .max(MAX_NOTE, t("products.movements.return.note_max")),
          refund: z.object({
            enabled: z.boolean(),
            amount: optionalPriceField(t),
            method: z.enum(REFUND_METHODS),
            cash_account_uuid: z.string(),
          }),
        })
        .superRefine((values, ctx) => {
          if (!values.refund.enabled) return;
          if (!values.refund.cash_account_uuid) {
            ctx.addIssue({
              code: "custom",
              path: ["refund", "cash_account_uuid"],
              message: t("products.validation.account_required"),
            });
          }
          const value = parseAmount(values.refund.amount);
          if (values.refund.amount.trim() && value > refundable + 0.0001) {
            ctx.addIssue({
              code: "custom",
              path: ["refund", "amount"],
              message: t("products.validation.amount_over_refundable"),
            });
          }
        }),
    [refundable, t],
  );

  return (
    <AppDialog
      open={Boolean(movement)}
      onOpenChange={onOpenChange}
      title={t("products.movements.return.title")}
      description={
        movement
          ? t("products.movements.return.description", {
              quantity: Math.abs(movement.quantity).toLocaleString(locale),
              product: movement.variant
                ? `${movement.product.name} · ${movement.variant.name}`
                : movement.product.name,
              branch: movement.branch.name,
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
          <Button type="submit" form={FORM_ID} disabled={returnSale.isPending}>
            {t("products.movements.return.submit")}
          </Button>
        </>
      }
    >
      {movement ? (
        <AppForm<ReturnFormValues>
          key={`${movement.uuid}-${canRefund ? "refund" : "plain"}`}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            reason: "",
            note: "",
            refund: {
              enabled: canRefund,
              amount: canRefund ? (view?.refundable ?? "") : "",
              method: "cash",
              cash_account_uuid: view?.refund_accounts[0]?.uuid ?? "",
            },
          }}
          onSubmit={async (values) => {
            const amount = normalizePrice(values.refund.amount);
            try {
              await returnSale.mutateAsync({
                uuid: movement.uuid,
                body: {
                  reason: values.reason.trim(),
                  ...(values.note.trim() ? { note: values.note.trim() } : {}),
                  ...(canRefund && values.refund.enabled
                    ? {
                        refund: {
                          method: values.refund.method,
                          cash_account_uuid: values.refund.cash_account_uuid,
                          ...(amount ? { amount } : {}),
                        },
                      }
                    : {}),
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
            name="reason"
            label={t("products.movements.return.reason")}
          />
          <AppTextarea
            name="note"
            label={t("products.movements.return.note")}
          />
          {canRefund && view ? (
            <RefundFields view={view} />
          ) : (
            <Alert>
              <AlertDescription>
                {refundable > 0
                  ? t("products.movements.return.money_later_hint", {
                      amount: formatAmount(
                        view?.refundable,
                        view?.currency,
                        locale,
                      ),
                    })
                  : t("products.movements.return.money_hint")}
              </AlertDescription>
            </Alert>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** Money paid back with the return: amount, way and account. */
function RefundFields({ view }: { view: StockSalePayments }) {
  const { t, locale } = useLocale();
  const form = useFormContext<ReturnFormValues>();
  const enabled = useWatch({ control: form.control, name: "refund.enabled" });
  const options = useMemo(
    () => accountOptions(view.refund_accounts, t),
    [t, view.refund_accounts],
  );

  // Switching the refund off clears a stale amount error.
  useEffect(() => {
    if (!enabled) form.clearErrors("refund");
  }, [enabled, form]);

  return (
    <div className="grid gap-4">
      <Separator />
      <AppSwitch
        name="refund.enabled"
        label={t("products.movements.return.refund_toggle")}
        description={t("products.movements.return.refund_toggle_hint", {
          amount: formatAmount(view.refundable, view.currency, locale),
        })}
      />
      {enabled ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <AppInput
            name="refund.amount"
            inputMode="decimal"
            label={t("products.movements.collection.amount")}
            description={t("products.movements.return.refund_amount_hint")}
          />
          <AppSelect
            name="refund.cash_account_uuid"
            label={t("products.movements.return.refund_account")}
            placeholder={t("products.movements.collection.account_placeholder")}
            options={options}
          />
          <AppToggleGroup
            name="refund.method"
            label={t("products.movements.collection.method")}
            options={REFUND_METHODS.map((method) => ({
              value: method,
              label: t(`products.movements.methods.${method}`),
            }))}
            className="sm:col-span-2"
          />
        </div>
      ) : null}
    </div>
  );
}

"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useReturnSale } from "@/features/products/hooks/use-products";
import type { StockMovement } from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "stock-sale-return-form";
const MAX_REASON = 200;
const MAX_NOTE = 2000;

/**
 * A manager takes back the product of a sale: every unit returns to the
 * branch stock and the sale's revenue is reversed. The money paid for it
 * is refunded separately.
 */
export function SaleReturnDialog({
  movement,
  onOpenChange,
}: {
  movement: StockMovement | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const returnSale = useReturnSale();
  const schema = useMemo(
    () =>
      z.object({
        reason: z
          .string()
          .trim()
          .min(1, t("products.movements.return.reason_required"))
          .max(MAX_REASON, t("products.movements.return.reason_max")),
        note: z.string().max(MAX_NOTE, t("products.movements.return.note_max")),
      }),
    [t],
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
        <AppForm<{ reason: string; note: string }>
          key={movement.uuid}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ reason: "", note: "" }}
          onSubmit={async (values) => {
            await returnSale.mutateAsync({
              uuid: movement.uuid,
              body: {
                reason: values.reason.trim(),
                ...(values.note.trim() ? { note: values.note.trim() } : {}),
              },
            });
            onOpenChange(false);
          }}
        >
          <Alert>
            <AlertDescription>
              {t("products.movements.return.money_hint")}
            </AlertDescription>
          </Alert>
          <AppInput
            name="reason"
            label={t("products.movements.return.reason")}
          />
          <AppTextarea
            name="note"
            label={t("products.movements.return.note")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

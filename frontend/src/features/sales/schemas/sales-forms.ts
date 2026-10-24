import { z } from "zod";

import { normalizeAmount } from "@/features/cash/lib/format";
import { MAX_QUANTITY } from "@/features/sales/constants";
import type { SaleLine, SaleLineCorrectRequest } from "@/features/sales/types";
import { toMinor } from "@/lib/utils/format";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const AMOUNT = /^\d+(?:[.,]\d{1,2})?$/;

function minorOf(value: string) {
  return toMinor(normalizeAmount(value)) ?? 0;
}

/**
 * Correction of a sale line: quantity, unit price and discount (the line
 * total stays ≥ the discount) and the reason that stays on both new lines.
 */
export function createCorrectLineSchema(t: Translate, line: SaleLine) {
  return z
    .object({
      quantity: z
        .string()
        .trim()
        .regex(/^\d+$/, t("sales.validation.quantity"))
        .refine((value) => {
          const n = Number(value);
          return n >= 1 && n <= MAX_QUANTITY;
        }, t("sales.validation.quantity")),
      unit_price: z.string().trim().regex(AMOUNT, t("sales.validation.amount")),
      discount_amount: z
        .string()
        .trim()
        .regex(AMOUNT, t("sales.validation.amount")),
      reason: z
        .string()
        .trim()
        .min(3, t("sales.validation.reason_min"))
        .max(500, t("sales.validation.reason_max")),
    })
    .superRefine((values, ctx) => {
      if (!AMOUNT.test(values.unit_price) || !/^\d+$/.test(values.quantity)) {
        return;
      }
      const gross = minorOf(values.unit_price) * Number(values.quantity);
      if (
        AMOUNT.test(values.discount_amount) &&
        minorOf(values.discount_amount) > gross
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["discount_amount"],
          message: t("sales.validation.discount_too_large"),
        });
      }
      if (!correctLineToRequest(line, values).changed) {
        ctx.addIssue({
          code: "custom",
          path: ["quantity"],
          message: t("sales.validation.nothing_changed"),
        });
      }
    });
}

export type CorrectLineFormValues = z.infer<
  ReturnType<typeof createCorrectLineSchema>
>;

export function correctLineDefaults(line: SaleLine): CorrectLineFormValues {
  return {
    quantity: String(line.quantity),
    unit_price: line.unit_price,
    discount_amount: line.discount_amount,
    reason: "",
  };
}

/**
 * Only the changed values go to the API: a field that is sent needs its
 * permission (`unit_price` → price override, `discount_amount` → discount).
 */
export function correctLineToRequest(
  line: SaleLine,
  values: CorrectLineFormValues,
): { body: SaleLineCorrectRequest; changed: boolean } {
  const body: SaleLineCorrectRequest = { reason: values.reason.trim() };
  const quantity = Number(values.quantity);
  if (quantity !== line.quantity) body.quantity = quantity;
  if (minorOf(values.unit_price) !== minorOf(line.unit_price)) {
    body.unit_price = normalizeAmount(values.unit_price);
  }
  if (minorOf(values.discount_amount) !== minorOf(line.discount_amount)) {
    body.discount_amount = normalizeAmount(values.discount_amount);
  }
  const changed =
    body.quantity !== undefined ||
    body.unit_price !== undefined ||
    body.discount_amount !== undefined;
  return { body, changed };
}

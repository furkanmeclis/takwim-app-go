import { z } from "zod";

import {
  MAX_QUANTITY,
  MOVEMENT_REQUEST_KINDS,
} from "@/features/products/constants";
import {
  normalizePrice,
  optionalPriceField,
} from "@/features/products/schemas/product-form";
import type {
  MovementRequestKind,
  StockMovementRequest,
} from "@/features/products/types";

type Translate = (key: string) => string;

/** Value of an empty staff pick. */
export const NO_STAFF = "none";

function optionalCount(t: Translate, min: number) {
  return z
    .string()
    .trim()
    .refine((v) => {
      if (v === "") return true;
      const n = Number(v);
      return Number.isInteger(n) && n >= min && n <= MAX_QUANTITY;
    }, t("products.validation.quantity"));
}

/**
 * One form for every kind; the fields a kind does not take are hidden and
 * left out of the request. Rules the API repeats (stock never below zero,
 * the branch selling the product) come back as field errors.
 */
export function createMovementSchema(t: Translate) {
  return z
    .object({
      kind: z.enum(MOVEMENT_REQUEST_KINDS as [string, ...string[]]),
      branch_uuid: z.string().min(1, t("products.validation.branch_required")),
      to_branch_uuid: z.string(),
      product_uuid: z
        .string()
        .min(1, t("products.validation.product_required")),
      variant_uuid: z.string(),
      quantity: optionalCount(t, 1),
      counted_quantity: optionalCount(t, 0),
      unit_price: optionalPriceField(t),
      unit_cost: optionalPriceField(t),
      staff_uuid: z.string(),
      reason: z.string().trim().max(200, t("products.validation.reason_max")),
      note: z.string().max(2000),
      occurred_at: z.string(),
    })
    .superRefine((values, ctx) => {
      if (values.kind === "adjustment") {
        if (values.counted_quantity === "") {
          ctx.addIssue({
            code: "custom",
            path: ["counted_quantity"],
            message: t("products.validation.counted_required"),
          });
        }
      } else if (values.quantity === "") {
        ctx.addIssue({
          code: "custom",
          path: ["quantity"],
          message: t("products.validation.quantity"),
        });
      }
      if (values.kind === "transfer") {
        if (!values.to_branch_uuid) {
          ctx.addIssue({
            code: "custom",
            path: ["to_branch_uuid"],
            message: t("products.validation.to_branch_required"),
          });
        } else if (values.to_branch_uuid === values.branch_uuid) {
          ctx.addIssue({
            code: "custom",
            path: ["to_branch_uuid"],
            message: t("products.errors.same_branch"),
          });
        }
      }
    });
}

export type MovementFormValues = z.infer<
  ReturnType<typeof createMovementSchema>
>;

export function movementFormDefaults(defaults: {
  kind?: MovementRequestKind;
  branchUuid?: string;
  productUuid?: string;
  variantUuid?: string;
}): MovementFormValues {
  return {
    kind: defaults.kind ?? "receipt",
    branch_uuid: defaults.branchUuid ?? "",
    to_branch_uuid: "",
    product_uuid: defaults.productUuid ?? "",
    variant_uuid: defaults.variantUuid ?? "",
    quantity: "",
    counted_quantity: "",
    unit_price: "",
    unit_cost: "",
    staff_uuid: NO_STAFF,
    reason: "",
    note: "",
    occurred_at: "",
  };
}

/** Local `yyyy-MM-dd'T'HH:mm` from the date picker → RFC 3339. */
function localToISO(value: string): string | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

export function movementFormToRequest(
  values: MovementFormValues,
): StockMovementRequest {
  const kind = values.kind as MovementRequestKind;
  const body: StockMovementRequest = {
    kind,
    branch_uuid: values.branch_uuid,
    product_uuid: values.product_uuid,
  };
  if (values.variant_uuid) body.variant_uuid = values.variant_uuid;
  if (kind === "adjustment") {
    body.counted_quantity = Number(values.counted_quantity);
  } else {
    body.quantity = Number(values.quantity);
  }
  if (kind === "transfer") body.to_branch_uuid = values.to_branch_uuid;
  if (kind === "sale" || kind === "return") {
    const price = normalizePrice(values.unit_price);
    if (price) body.unit_price = price;
    if (values.staff_uuid && values.staff_uuid !== NO_STAFF) {
      body.staff_uuid = values.staff_uuid;
    }
  }
  if (kind === "receipt") {
    const cost = normalizePrice(values.unit_cost);
    if (cost) body.unit_cost = cost;
  }
  if (values.reason.trim()) body.reason = values.reason.trim();
  if (values.note.trim()) body.note = values.note.trim();
  const occurred = localToISO(values.occurred_at);
  if (occurred) body.occurred_at = occurred;
  return body;
}

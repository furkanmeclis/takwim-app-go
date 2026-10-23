import { z } from "zod";

import {
  MAX_LOW_STOCK,
  MAX_SORT_ORDER,
  PRODUCT_UNITS,
} from "@/features/products/constants";
import type {
  Product,
  ProductCreateRequest,
  ProductUnit,
  ProductVariant,
  ProductVariantCreateRequest,
} from "@/features/products/types";

type Translate = (key: string) => string;

const AMOUNT = /^\d+([.,]\d{1,2})?$/;
const CODE = /^\S+$/;

export function intField(
  t: Translate,
  min: number,
  max: number,
  messageKey: string,
) {
  return z
    .string()
    .trim()
    .refine((v) => {
      const n = Number(v);
      return v !== "" && Number.isInteger(n) && n >= min && n <= max;
    }, t(messageKey));
}

export function priceField(t: Translate) {
  return z
    .string()
    .trim()
    .refine((v) => AMOUNT.test(v), t("products.validation.price"));
}

export function optionalPriceField(t: Translate) {
  return z
    .string()
    .trim()
    .refine((v) => v === "" || AMOUNT.test(v), t("products.validation.price"));
}

function skuField(t: Translate) {
  return z
    .string()
    .trim()
    .min(1, t("products.validation.sku_required"))
    .max(64, t("products.validation.sku_max"))
    .refine((v) => CODE.test(v), t("products.validation.sku_format"));
}

function barcodeField(t: Translate) {
  return z.string().trim().max(64, t("products.validation.barcode_max"));
}

/** "450,5" → "450.50"; "" → null. */
export function normalizePrice(value: string): string | null {
  const v = value.trim().replace(",", ".");
  if (!v) return null;
  return Number(v).toFixed(2);
}

export function createProductFormSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("products.validation.name_required"))
      .max(160, t("products.validation.name_max")),
    sku: skuField(t),
    barcode: barcodeField(t),
    unit: z.enum(PRODUCT_UNITS as [string, ...string[]]),
    description: z.string().max(2000),
    price: priceField(t),
    cost: optionalPriceField(t),
    low_stock_threshold: intField(
      t,
      0,
      MAX_LOW_STOCK,
      "products.validation.low_stock",
    ),
    branch_uuids: z.array(z.string()),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type ProductFormValues = z.infer<
  ReturnType<typeof createProductFormSchema>
>;

export function productFormDefaults(
  product?: Product,
  defaults?: { branchUuids?: string[] },
): ProductFormValues {
  return {
    name: product?.name ?? "",
    sku: product?.sku ?? "",
    barcode: product?.barcode ?? "",
    unit: product?.unit ?? "piece",
    description: product?.description ?? "",
    price: product?.price ?? "",
    cost: product && product.cost !== "0.00" ? product.cost : "",
    low_stock_threshold: String(product?.low_stock_threshold ?? 0),
    branch_uuids: defaults?.branchUuids ?? [],
    show_online: product?.show_online ?? true,
    is_active: product?.is_active ?? true,
  };
}

/** Create body; edit drops `branch_uuids` (offers change on the detail page). */
export function productFormToRequest(
  values: ProductFormValues,
): ProductCreateRequest {
  return {
    name: values.name.trim(),
    sku: values.sku.trim(),
    barcode: values.barcode.trim(),
    unit: values.unit as ProductUnit,
    description: values.description.trim(),
    price: normalizePrice(values.price) ?? "0.00",
    cost: normalizePrice(values.cost) ?? "0.00",
    low_stock_threshold: Number(values.low_stock_threshold),
    show_online: values.show_online,
    is_active: values.is_active,
    branch_uuids: values.branch_uuids,
  };
}

export function createQuickEditSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("products.validation.name_required"))
      .max(160, t("products.validation.name_max")),
    price: priceField(t),
    low_stock_threshold: intField(
      t,
      0,
      MAX_LOW_STOCK,
      "products.validation.low_stock",
    ),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

/** Branch offer dialog: empty price = catalog price. */
export function createOfferSchema(t: Translate) {
  return z.object({
    branch_uuid: z.string().min(1, t("products.validation.branch_required")),
    price: optionalPriceField(t),
    is_active: z.boolean(),
  });
}

export type OfferFormValues = z.infer<ReturnType<typeof createOfferSchema>>;

/** Variant dialog: empty price / cost = the product's. */
export function createVariantSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("products.validation.name_required"))
      .max(120, t("products.validation.variant_name_max")),
    sku: skuField(t),
    barcode: barcodeField(t),
    price: optionalPriceField(t),
    cost: optionalPriceField(t),
    sort_order: intField(
      t,
      0,
      MAX_SORT_ORDER,
      "products.validation.sort_order",
    ),
    is_active: z.boolean(),
  });
}

export type VariantFormValues = z.infer<ReturnType<typeof createVariantSchema>>;

export function variantFormDefaults(
  variant: ProductVariant | null,
  next: number,
): VariantFormValues {
  return {
    name: variant?.name ?? "",
    sku: variant?.sku ?? "",
    barcode: variant?.barcode ?? "",
    price: variant?.price ?? "",
    cost: variant?.cost ?? "",
    sort_order: String(variant?.sort_order ?? next),
    is_active: variant?.is_active ?? true,
  };
}

export function variantFormToRequest(
  values: VariantFormValues,
): ProductVariantCreateRequest {
  return {
    name: values.name.trim(),
    sku: values.sku.trim(),
    barcode: values.barcode.trim(),
    price: normalizePrice(values.price),
    cost: normalizePrice(values.cost),
    sort_order: Number(values.sort_order),
    is_active: values.is_active,
  };
}

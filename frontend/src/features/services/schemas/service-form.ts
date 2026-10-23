import { z } from "zod";

import {
  MAX_BUFFER,
  MAX_DURATION,
  MAX_SORT_ORDER,
  MIN_DURATION,
  PAYMENT_MODES,
  STAFF_MODES,
  UNCATEGORIZED,
} from "@/features/services/constants";
import type {
  PaymentMode,
  Service,
  ServiceCreateRequest,
  StaffMode,
} from "@/features/services/types";

type Translate = (key: string) => string;

const AMOUNT = /^\d+([.,]\d{1,2})?$/;

function intField(t: Translate, min: number, max: number, messageKey: string) {
  return z
    .string()
    .trim()
    .refine((v) => {
      const n = Number(v);
      return v !== "" && Number.isInteger(n) && n >= min && n <= max;
    }, t(messageKey));
}

/** Empty = "use the catalog value". */
function optionalIntField(
  t: Translate,
  min: number,
  max: number,
  messageKey: string,
) {
  return z
    .string()
    .trim()
    .refine((v) => {
      if (v === "") return true;
      const n = Number(v);
      return Number.isInteger(n) && n >= min && n <= max;
    }, t(messageKey));
}

export function priceField(t: Translate) {
  return z
    .string()
    .trim()
    .refine((v) => AMOUNT.test(v), t("services.validation.price"));
}

export function optionalPriceField(t: Translate) {
  return z
    .string()
    .trim()
    .refine((v) => v === "" || AMOUNT.test(v), t("services.validation.price"));
}

/** "450,5" → "450.50"; "" → null. */
export function normalizePrice(value: string): string | null {
  const v = value.trim().replace(",", ".");
  if (!v) return null;
  return Number(v).toFixed(2);
}

export function nullableInt(value: string): number | null {
  return value.trim() === "" ? null : Number(value);
}

export function createServiceFormSchema(t: Translate) {
  return z.object({
    category_uuid: z.string(),
    name: z
      .string()
      .trim()
      .min(1, t("services.validation.name_required"))
      .max(160, t("services.validation.name_max")),
    description: z.string().max(2000),
    duration_minutes: intField(
      t,
      MIN_DURATION,
      MAX_DURATION,
      "services.validation.duration",
    ),
    buffer_before_minutes: intField(
      t,
      0,
      MAX_BUFFER,
      "services.validation.buffer",
    ),
    buffer_after_minutes: intField(
      t,
      0,
      MAX_BUFFER,
      "services.validation.buffer",
    ),
    price: priceField(t),
    payment_mode: z.enum(PAYMENT_MODES as [string, ...string[]]),
    staff_mode: z.enum(STAFF_MODES as [string, ...string[]]),
    resource_kinds: z.array(z.string()),
    branch_uuids: z.array(z.string()),
    sort_order: intField(
      t,
      0,
      MAX_SORT_ORDER,
      "services.validation.sort_order",
    ),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type ServiceFormValues = z.infer<
  ReturnType<typeof createServiceFormSchema>
>;

export function serviceFormDefaults(
  service?: Service,
  defaults?: { staffMode?: StaffMode; branchUuids?: string[] },
): ServiceFormValues {
  return {
    category_uuid: service?.category?.uuid ?? UNCATEGORIZED,
    name: service?.name ?? "",
    description: service?.description ?? "",
    duration_minutes: String(service?.duration_minutes ?? 30),
    buffer_before_minutes: String(service?.buffer_before_minutes ?? 0),
    buffer_after_minutes: String(service?.buffer_after_minutes ?? 0),
    price: service?.price ?? "",
    payment_mode: service?.payment_mode ?? "on_site",
    staff_mode: service?.staff_mode ?? defaults?.staffMode ?? "required",
    resource_kinds: service?.resource_kinds ?? [],
    branch_uuids: defaults?.branchUuids ?? [],
    sort_order: String(service?.sort_order ?? 0),
    show_online: service?.show_online ?? true,
    is_active: service?.is_active ?? true,
  };
}

/** Create body; edit drops `branch_uuids` (offers change on the detail page). */
export function serviceFormToRequest(
  values: ServiceFormValues,
): ServiceCreateRequest {
  return {
    category_uuid:
      values.category_uuid === UNCATEGORIZED ? null : values.category_uuid,
    name: values.name.trim(),
    description: values.description.trim(),
    duration_minutes: Number(values.duration_minutes),
    buffer_before_minutes: Number(values.buffer_before_minutes),
    buffer_after_minutes: Number(values.buffer_after_minutes),
    price: normalizePrice(values.price) ?? "0.00",
    payment_mode: values.payment_mode as PaymentMode,
    staff_mode: values.staff_mode as StaffMode,
    resource_kinds:
      values.resource_kinds as ServiceCreateRequest["resource_kinds"],
    sort_order: Number(values.sort_order),
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
      .min(1, t("services.validation.name_required"))
      .max(160, t("services.validation.name_max")),
    duration_minutes: intField(
      t,
      MIN_DURATION,
      MAX_DURATION,
      "services.validation.duration",
    ),
    price: priceField(t),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

/** Branch offer dialog: empty price / duration = catalog value. */
export function createOfferSchema(t: Translate) {
  return z.object({
    branch_uuid: z.string().min(1, t("services.validation.branch_required")),
    price: optionalPriceField(t),
    duration_minutes: optionalIntField(
      t,
      MIN_DURATION,
      MAX_DURATION,
      "services.validation.duration",
    ),
    is_active: z.boolean(),
  });
}

export type OfferFormValues = z.infer<ReturnType<typeof createOfferSchema>>;

/** Staff link dialog: empty price / duration = the service's. */
export function createStaffLinkSchema(t: Translate) {
  return z.object({
    staff_uuid: z.string().min(1, t("services.validation.staff_required")),
    price: optionalPriceField(t),
    duration_minutes: optionalIntField(
      t,
      MIN_DURATION,
      MAX_DURATION,
      "services.validation.duration",
    ),
  });
}

export type StaffLinkFormValues = z.infer<
  ReturnType<typeof createStaffLinkSchema>
>;

export function createCategorySchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("services.validation.name_required"))
      .max(120, t("services.validation.category_name_max")),
    description: z.string().max(2000),
    sort_order: intField(
      t,
      0,
      MAX_SORT_ORDER,
      "services.validation.sort_order",
    ),
  });
}

export type CategoryFormValues = z.infer<
  ReturnType<typeof createCategorySchema>
>;

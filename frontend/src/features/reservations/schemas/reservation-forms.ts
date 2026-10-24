import { z } from "zod";

import {
  MAX_LINES,
  MAX_NOTE,
  MAX_PARTY_SIZE,
  MAX_PLATE,
  MIN_PARTY_SIZE,
  VEHICLE_TYPES,
  type VehicleType,
} from "@/features/reservations/constants";
import {
  customFieldIssues,
  type CustomFieldValues,
  type FormQuestion,
} from "@/features/reservations/lib/custom-fields";
import type { ReservationFormField } from "@/features/reservations/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Answers of the booking questions asked in the panel (TEC-905). */
const customFieldsShape = z.record(
  z.string(),
  z.union([z.string(), z.boolean()]),
);

function refineCustomFields(
  t: Translate,
  fields: FormQuestion[],
  values: CustomFieldValues,
  ctx: z.RefinementCtx,
) {
  for (const issue of customFieldIssues(fields, values, t)) {
    ctx.addIssue({
      code: "custom",
      path: ["custom_fields", issue.key],
      message: issue.message,
    });
  }
}

/** Cancel reasons offered in the panel (the API takes any short code). */
export const CANCEL_REASONS = ["customer_request", "business"] as const;

export function createActionSchema(t: Translate) {
  return z.object({
    reason: z.string(),
    note: z.string().max(MAX_NOTE, t("reservations.validation.note_max")),
  });
}

export type ActionFormValues = z.infer<ReturnType<typeof createActionSchema>>;

/**
 * Guests as typed. Required (a whole number in range) when the sector books
 * by party size; otherwise empty or a valid number.
 */
function partySizeField(t: Translate, required: boolean) {
  return z
    .string()
    .trim()
    .refine(
      (value) =>
        (!required && value === "") ||
        (/^\d+$/.test(value) &&
          Number(value) >= MIN_PARTY_SIZE &&
          Number(value) <= MAX_PARTY_SIZE),
      t("reservations.validation.party_size"),
    );
}

export function createBookingSchema(
  t: Translate,
  {
    party = false,
    fields = [],
  }: { party?: boolean; fields?: ReservationFormField[] } = {},
) {
  return z
    .object({
      branch_uuid: z
        .string()
        .min(1, t("reservations.validation.branch_required")),
      customer_uuid: z
        .string()
        .min(1, t("reservations.validation.customer_required")),
      /** NO_BUNDLE or a combined service bundle (TEC-895). */
      bundle_uuid: z.string(),
      lines: z
        .array(
          z.object({
            service_uuid: z
              .string()
              .min(1, t("reservations.validation.service_required")),
            /** ANY_STAFF = anyone free. */
            staff_uuid: z.string(),
            /** NO_PACKAGE or the customer's package paying the line (TEC-984). */
            package_uuid: z.string(),
          }),
        )
        .min(1, t("reservations.validation.service_required"))
        .max(MAX_LINES),
      party_size: partySizeField(t, party),
      /** ANY_ZONE = any area of the branch. */
      zone: z.string(),
      /** Car wash: NO_VEHICLE or a vehicle type, and the plate. */
      vehicle_type: z.string(),
      vehicle_plate: z
        .string()
        .trim()
        .max(MAX_PLATE, t("reservations.validation.plate"))
        .regex(/^[\p{L}\p{N} ]*$/u, t("reservations.validation.plate")),
      date: z.string().min(1, t("reservations.validation.date_required")),
      /** ISO instant of the picked slot. */
      starts_at: z.string().min(1, t("reservations.validation.slot_required")),
      customer_note: z
        .string()
        .max(MAX_NOTE, t("reservations.validation.note_max")),
      internal_note: z
        .string()
        .max(MAX_NOTE, t("reservations.validation.note_max")),
      custom_fields: customFieldsShape,
    })
    .superRefine((values, ctx) =>
      refineCustomFields(t, fields, values.custom_fields, ctx),
    );
}

export type BookingFormValues = z.infer<ReturnType<typeof createBookingSchema>>;

export function createNotesSchema(
  t: Translate,
  fields: ReservationFormField[] = [],
) {
  return z
    .object({
      customer_note: z
        .string()
        .max(MAX_NOTE, t("reservations.validation.note_max")),
      internal_note: z
        .string()
        .max(MAX_NOTE, t("reservations.validation.note_max")),
      custom_fields: customFieldsShape,
    })
    .superRefine((values, ctx) =>
      refineCustomFields(t, fields, values.custom_fields, ctx),
    );
}

export type NotesFormValues = z.infer<ReturnType<typeof createNotesSchema>>;

/** Ways of an on-site payment (online comes only from the provider). */
export const COLLECT_METHODS = ["cash", "card", "transfer"] as const;

/** Most characters of a collection's note (API limit). */
export const MAX_COLLECT_NOTE = 240;

const AMOUNT = /^\d+([.,]\d{1,2})?$/;

/** "450,5" → 450.5 (NaN when not an amount). */
export const parseAmount = (value: string) =>
  AMOUNT.test(value.trim()) ? Number(value.trim().replace(",", ".")) : NaN;

/** On-site payment: at most the remaining amount (the API checks again). */
export function createCollectSchema(t: Translate, remaining: number) {
  return z.object({
    amount: z
      .string()
      .trim()
      .refine((v) => parseAmount(v) > 0, t("reservations.validation.amount"))
      .refine(
        (v) => !(parseAmount(v) > remaining + 0.0001),
        t("reservations.validation.amount_over_remaining"),
      ),
    method: z.enum(COLLECT_METHODS),
    cash_account_uuid: z
      .string()
      .min(1, t("reservations.validation.account_required")),
    note: z
      .string()
      .max(MAX_COLLECT_NOTE, t("reservations.validation.collect_note_max")),
  });
}

export type CollectFormValues = z.infer<ReturnType<typeof createCollectSchema>>;

/** Ways of a refund recorded by hand ("online" = paid back outside Takwim). */
export const REFUND_METHODS = ["cash", "card", "transfer", "online"] as const;
export const REFUND_CHANNELS = ["provider", "manual"] as const;

/** Most characters of a refund's reason or a resolution note (API limit). */
export const MAX_REFUND_NOTE = 240;

/**
 * Refund: at most what is left of the chosen collection (the API checks
 * again under a lock); a manual refund names its way and account.
 */
export function createRefundSchema(
  t: Translate,
  refundableOf: (collection: string) => number,
) {
  return z
    .object({
      collection_uuid: z
        .string()
        .min(1, t("reservations.validation.collection_required")),
      amount: z.string().trim(),
      channel: z.enum(REFUND_CHANNELS),
      method: z.enum(REFUND_METHODS),
      cash_account_uuid: z.string(),
      reason: z
        .string()
        .max(MAX_REFUND_NOTE, t("reservations.validation.collect_note_max")),
    })
    .superRefine((values, ctx) => {
      const value = parseAmount(values.amount);
      if (!(value > 0)) {
        ctx.addIssue({
          code: "custom",
          path: ["amount"],
          message: t("reservations.validation.amount"),
        });
      } else if (value > refundableOf(values.collection_uuid) + 0.0001) {
        ctx.addIssue({
          code: "custom",
          path: ["amount"],
          message: t("reservations.validation.amount_over_refundable"),
        });
      }
      if (values.channel === "manual" && !values.cash_account_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["cash_account_uuid"],
          message: t("reservations.validation.account_required"),
        });
      }
    });
}

export type RefundFormValues = z.infer<ReturnType<typeof createRefundSchema>>;

/** Select value for "anyone free" (Radix Select has no empty value). */
export const ANY_STAFF = "any";

/** Select value for "any area" of the branch. */
export const ANY_ZONE = "any";

/** Select value for "no bundle": the services are picked one by one. */
export const NO_BUNDLE = "none";

/** Select value for "pay the line" (no package session). */
export const NO_PACKAGE = "none";

/** Lines paid with a package session, for the API (TEC-984). */
export function toPackageSessions(lines: { package_uuid?: string }[]) {
  return lines.flatMap((line, index) =>
    line.package_uuid && line.package_uuid !== NO_PACKAGE
      ? [{ customer_package_uuid: line.package_uuid, line_index: index }]
      : [],
  );
}

/** Select value for "no vehicle given" (car wash). */
export const NO_VEHICLE = "none";

/** The vehicle for the API (undefined when no type was picked). */
export function toVehicle(type: string, plate: string) {
  if (!VEHICLE_TYPES.includes(type as VehicleType)) return undefined;
  return { type: type as VehicleType, plate: plate.trim() };
}

/** Typed guests for the API (undefined when empty or not a valid count). */
export function toPartySize(value: string) {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return undefined;
  const count = Number(trimmed);
  return count >= MIN_PARTY_SIZE && count <= MAX_PARTY_SIZE ? count : undefined;
}

/** ANY_ZONE = no area asked (undefined for the API). */
export function toZone(value: string) {
  return value && value !== ANY_ZONE ? value : undefined;
}

/** ANY_STAFF = anyone free (null for the API). */
export function toLineInputs(
  lines: { service_uuid: string; staff_uuid: string }[],
) {
  return lines.map((line) => ({
    service_uuid: line.service_uuid,
    staff_uuid:
      line.staff_uuid && line.staff_uuid !== ANY_STAFF ? line.staff_uuid : null,
  }));
}

export function createRescheduleSchema(
  t: Translate,
  { party = false }: { party?: boolean } = {},
) {
  return z.object({
    lines: z.array(
      z.object({ service_uuid: z.string(), staff_uuid: z.string() }),
    ),
    party_size: partySizeField(t, party),
    date: z.string().min(1, t("reservations.validation.date_required")),
    starts_at: z.string().min(1, t("reservations.validation.slot_required")),
    note: z.string().max(MAX_NOTE, t("reservations.validation.note_max")),
  });
}

export type RescheduleFormValues = z.infer<
  ReturnType<typeof createRescheduleSchema>
>;

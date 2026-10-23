import { z } from "zod";

import { MAX_LINES, MAX_NOTE } from "@/features/reservations/constants";

type Translate = (key: string) => string;

/** Cancel reasons offered in the panel (the API takes any short code). */
export const CANCEL_REASONS = ["customer_request", "business"] as const;

export function createActionSchema(t: Translate) {
  return z.object({
    reason: z.string(),
    note: z.string().max(MAX_NOTE, t("reservations.validation.note_max")),
  });
}

export type ActionFormValues = z.infer<ReturnType<typeof createActionSchema>>;

export function createBookingSchema(t: Translate) {
  return z.object({
    branch_uuid: z
      .string()
      .min(1, t("reservations.validation.branch_required")),
    customer_uuid: z
      .string()
      .min(1, t("reservations.validation.customer_required")),
    lines: z
      .array(
        z.object({
          service_uuid: z
            .string()
            .min(1, t("reservations.validation.service_required")),
          /** ANY_STAFF = anyone free. */
          staff_uuid: z.string(),
        }),
      )
      .min(1, t("reservations.validation.service_required"))
      .max(MAX_LINES),
    date: z.string().min(1, t("reservations.validation.date_required")),
    /** ISO instant of the picked slot. */
    starts_at: z.string().min(1, t("reservations.validation.slot_required")),
    customer_note: z
      .string()
      .max(MAX_NOTE, t("reservations.validation.note_max")),
    internal_note: z
      .string()
      .max(MAX_NOTE, t("reservations.validation.note_max")),
  });
}

export type BookingFormValues = z.infer<ReturnType<typeof createBookingSchema>>;

export function createNotesSchema(t: Translate) {
  return z.object({
    customer_note: z
      .string()
      .max(MAX_NOTE, t("reservations.validation.note_max")),
    internal_note: z
      .string()
      .max(MAX_NOTE, t("reservations.validation.note_max")),
  });
}

export type NotesFormValues = z.infer<ReturnType<typeof createNotesSchema>>;

/** Select value for "anyone free" (Radix Select has no empty value). */
export const ANY_STAFF = "any";

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

export function createRescheduleSchema(t: Translate) {
  return z.object({
    lines: z.array(
      z.object({ service_uuid: z.string(), staff_uuid: z.string() }),
    ),
    date: z.string().min(1, t("reservations.validation.date_required")),
    starts_at: z.string().min(1, t("reservations.validation.slot_required")),
    note: z.string().max(MAX_NOTE, t("reservations.validation.note_max")),
  });
}

export type RescheduleFormValues = z.infer<
  ReturnType<typeof createRescheduleSchema>
>;

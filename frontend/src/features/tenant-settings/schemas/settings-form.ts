import { z } from "zod";

import {
  BOOKING_FIELD_TYPES,
  INVOICE_ITEM_TYPES,
  MAX_BOOKING_FIELDS,
  SALES_CHANNELS,
} from "@/features/tenant-settings/constants";
import {
  isValidTCKN,
  isValidVKN,
} from "@/features/tenant-settings/lib/tax-number";
import type {
  OrganizationSettings,
  OrganizationSettingsSaveRequest,
} from "@/features/tenant-settings/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const PERCENT = /^\d{1,3}(\.\d{1,2})?$/;

/** Most sites one business may embed on (API limit). */
export const MAX_EMBED_ORIGINS = 20;

/**
 * One line of the embed sites field: a site address (`example.com`,
 * `https://shop.example.com`, `https://*.example.com`; http only for
 * localhost). The API normalizes it again (TEC-908).
 */
const ORIGIN_LINE =
  /^(?:(?:https:\/\/)?(?:\*\.)?(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]*[a-z0-9]|(?:https?:\/\/)?(?:localhost|127\.0\.0\.1))(?::\d{1,5})?\/?$/i;

/** Non-empty lines of the embed sites field. */
export function originLines(text: string) {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}
const MINUTES = /^\d{1,6}$/;

function minutesField(t: Translate, min: number, max: number) {
  return z
    .string()
    .trim()
    .refine(
      (v) =>
        v === "" || (MINUTES.test(v) && Number(v) >= min && Number(v) <= max),
      t("tenant_settings.validation.minutes_range", { min, max }),
    );
}

function daysField(t: Translate, min: number, max: number) {
  return z
    .string()
    .trim()
    .refine(
      (v) =>
        v === "" || (MINUTES.test(v) && Number(v) >= min && Number(v) <= max),
      t("tenant_settings.validation.days_range", { min, max }),
    );
}

function countField(t: Translate, min: number, max: number) {
  return z
    .string()
    .trim()
    .refine(
      (v) =>
        v === "" || (MINUTES.test(v) && Number(v) >= min && Number(v) <= max),
      t("tenant_settings.validation.count_range", { min, max }),
    );
}

function stepField(t: Translate) {
  return z
    .string()
    .trim()
    .refine(
      (v) =>
        v === "" ||
        (MINUTES.test(v) &&
          Number(v) >= 5 &&
          Number(v) <= 240 &&
          Number(v) % 5 === 0),
      t("tenant_settings.validation.slot_step"),
    );
}

/** Choices of a select question: one per line, trimmed, empty lines out. */
export function optionLines(raw: string) {
  return raw
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function percentField(t: Translate) {
  return z
    .string()
    .trim()
    .refine(
      (v) => v === "" || (PERCENT.test(v) && Number(v) <= 100),
      t("tenant_settings.validation.percent"),
    );
}

export function createSettingsSchema(t: Translate) {
  return z
    .object({
      company: z.object({
        legal_name: z.string().max(255),
        tax_number: z
          .string()
          .trim()
          .refine((raw) => {
            const v = raw.replace(/\s/g, "");
            if (v === "") return true;
            if (v.length === 10) return isValidVKN(v);
            if (v.length === 11) return isValidTCKN(v);
            return false;
          }, t("tenant_settings.validation.tax_number")),
        tax_office: z.string().max(128),
        address: z.string().max(1000),
        phone: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || /^\+?[0-9 ()-]{7,32}$/.test(v),
            t("tenant_settings.validation.phone"),
          ),
        email: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || z.email().safeParse(v).success,
            t("tenant_settings.validation.email"),
          ),
        website: z
          .string()
          .trim()
          .max(255)
          .refine(
            (v) => v === "" || /^https?:\/\/[^\s/]+\.[^\s]+$/i.test(v),
            t("tenant_settings.validation.website"),
          ),
      }),
      reservation: z.object({
        approval_mode: z.enum(["", "auto", "staff"]),
        staff_review_window_minutes: minutesField(t, 1, 10080),
        payment_hold_minutes: minutesField(t, 1, 10080),
        min_lead_minutes: minutesField(t, 0, 43200),
        max_advance_days: daysField(t, 1, 730),
        cancel_cutoff_minutes: minutesField(t, 0, 43200),
        change_cutoff_minutes: minutesField(t, 0, 43200),
        slot_step_minutes: stepField(t),
        max_open_per_customer: countField(t, 1, 100),
        max_online_per_day: countField(t, 1, 10000),
        /** A no-show uses the visit's package sessions (TEC-984). */
        no_show_uses_package_session: z.boolean(),
      }),
      booking: z.object({
        show_staff: z.boolean(),
        show_products: z.boolean(),
        show_area: z.boolean(),
        fields: z
          .array(
            z.object({
              key: z.string(),
              label: z
                .string()
                .trim()
                .min(1, t("tenant_settings.validation.field_label"))
                .max(80),
              type: z.enum(BOOKING_FIELD_TYPES),
              options: z.string(),
              required: z.boolean(),
              online: z.boolean(),
              panel: z.boolean(),
              help: z.string().trim().max(200),
            }),
          )
          .max(MAX_BOOKING_FIELDS),
      }),
      payment: z.object({
        pay_at_venue: z.boolean(),
        pay_online: z.boolean(),
      }),
      policies: z.object({
        cancellation: z.string().max(5000),
        no_show: z.string().max(5000),
        change: z.string().max(5000),
      }),
      sales_channels: z.array(
        z.object({ code: z.string(), is_active: z.boolean() }),
      ),
      embed: z.object({
        allowed_origins: z.string().superRefine((text, ctx) => {
          const lines = originLines(text);
          const bad = lines.findIndex((line) => !ORIGIN_LINE.test(line));
          if (bad >= 0) {
            ctx.addIssue({
              code: "custom",
              message: t("tenant_settings.validation.embed_origin", {
                line: bad + 1,
              }),
            });
          } else if (lines.length > MAX_EMBED_ORIGINS) {
            ctx.addIssue({
              code: "custom",
              message: t("tenant_settings.validation.embed_origins_max", {
                max: MAX_EMBED_ORIGINS,
              }),
            });
          }
        }),
      }),
      tax: z.object({
        default_vat_rate: percentField(t),
        invoice_item_mappings: z.array(
          z.object({
            item_type: z.string(),
            account_code: z.string().max(64),
            vat_rate: percentField(t),
            description: z.string().max(255),
          }),
        ),
      }),
    })
    .superRefine((values, ctx) => {
      values.booking.fields.forEach((field, index) => {
        if (!field.online && !field.panel) {
          ctx.addIssue({
            code: "custom",
            path: ["booking", "fields", index, "online"],
            message: t("tenant_settings.validation.field_where"),
          });
        }
        if (field.type === "select") {
          const options = optionLines(field.options);
          if (options.length < 2 || new Set(options).size !== options.length) {
            ctx.addIssue({
              code: "custom",
              path: ["booking", "fields", index, "options"],
              message: t("tenant_settings.validation.field_options"),
            });
          }
        }
      });
      if (!values.payment.pay_at_venue && !values.payment.pay_online) {
        ctx.addIssue({
          code: "custom",
          path: ["payment", "pay_at_venue"],
          message: t("tenant_settings.validation.payment_option"),
        });
      }
      const { min_lead_minutes: lead, max_advance_days: days } =
        values.reservation;
      if (lead !== "" && days !== "" && Number(lead) > Number(days) * 1440) {
        ctx.addIssue({
          code: "custom",
          path: ["reservation", "min_lead_minutes"],
          message: t("tenant_settings.validation.lead_after_window"),
        });
      }
    });
}

export type SettingsFormValues = z.infer<
  ReturnType<typeof createSettingsSchema>
>;

const minutesToField = (v: number | null | undefined) =>
  v === null || v === undefined ? "" : String(v);
const fieldToMinutes = (v: string) => (v.trim() === "" ? null : Number(v));

/** API document → form values (numbers become strings, lists complete). */
export function settingsToForm(s: OrganizationSettings): SettingsFormValues {
  const channels = new Map(s.sales_channels.map((c) => [c.code, c.is_active]));
  const mappings = new Map(
    s.tax.invoice_item_mappings.map((m) => [m.item_type, m]),
  );
  return {
    company: { ...s.company },
    reservation: {
      approval_mode: s.reservation.approval_mode ?? "",
      staff_review_window_minutes: minutesToField(
        s.reservation.staff_review_window_minutes,
      ),
      payment_hold_minutes: minutesToField(s.reservation.payment_hold_minutes),
      min_lead_minutes: minutesToField(s.reservation.min_lead_minutes),
      max_advance_days: minutesToField(s.reservation.max_advance_days),
      cancel_cutoff_minutes: minutesToField(
        s.reservation.cancel_cutoff_minutes,
      ),
      change_cutoff_minutes: minutesToField(
        s.reservation.change_cutoff_minutes,
      ),
      slot_step_minutes: minutesToField(s.reservation.slot_step_minutes),
      max_open_per_customer: minutesToField(
        s.reservation.max_open_per_customer,
      ),
      max_online_per_day: minutesToField(s.reservation.max_online_per_day),
      no_show_uses_package_session: Boolean(
        s.reservation.no_show_uses_package_session,
      ),
    },
    booking: {
      show_staff: !s.booking.hidden_steps.includes("staff"),
      show_products: !s.booking.hidden_steps.includes("products"),
      show_area: !s.booking.hidden_steps.includes("area"),
      fields: s.booking.fields.map((f) => ({
        key: f.key,
        label: f.label,
        type: f.type,
        options: f.options.join("\n"),
        required: f.required,
        online: f.online,
        panel: f.panel,
        help: f.help,
      })),
    },
    payment: { ...s.payment },
    policies: { ...s.policies },
    sales_channels: SALES_CHANNELS.map((code) => ({
      code,
      is_active: channels.get(code) ?? false,
    })),
    embed: { allowed_origins: (s.embed?.allowed_origins ?? []).join("\n") },
    tax: {
      default_vat_rate: s.tax.default_vat_rate,
      invoice_item_mappings: INVOICE_ITEM_TYPES.map((item_type) => {
        const m = mappings.get(item_type);
        return {
          item_type,
          account_code: m?.account_code ?? "",
          vat_rate: m?.vat_rate ?? "",
          description: m?.description ?? "",
        };
      }),
    },
  };
}

/** Form values → PUT body (the API normalizes and validates again). */
export function formToSaveRequest(
  values: SettingsFormValues,
  baseVersion: number,
): OrganizationSettingsSaveRequest {
  return {
    base_version: baseVersion,
    settings: {
      company: {
        ...values.company,
        tax_number: values.company.tax_number.replace(/\s/g, ""),
      },
      reservation: {
        approval_mode: values.reservation.approval_mode,
        staff_review_window_minutes: fieldToMinutes(
          values.reservation.staff_review_window_minutes,
        ),
        payment_hold_minutes: fieldToMinutes(
          values.reservation.payment_hold_minutes,
        ),
        min_lead_minutes: fieldToMinutes(values.reservation.min_lead_minutes),
        max_advance_days: fieldToMinutes(values.reservation.max_advance_days),
        cancel_cutoff_minutes: fieldToMinutes(
          values.reservation.cancel_cutoff_minutes,
        ),
        change_cutoff_minutes: fieldToMinutes(
          values.reservation.change_cutoff_minutes,
        ),
        slot_step_minutes: fieldToMinutes(values.reservation.slot_step_minutes),
        max_open_per_customer: fieldToMinutes(
          values.reservation.max_open_per_customer,
        ),
        max_online_per_day: fieldToMinutes(
          values.reservation.max_online_per_day,
        ),
        no_show_uses_package_session:
          values.reservation.no_show_uses_package_session,
      },
      booking: {
        hidden_steps: [
          ...(values.booking.show_staff ? [] : (["staff"] as const)),
          ...(values.booking.show_products ? [] : (["products"] as const)),
          ...(values.booking.show_area ? [] : (["area"] as const)),
        ],
        fields: values.booking.fields.map((f) => ({
          key: f.key,
          label: f.label.trim(),
          type: f.type,
          options: f.type === "select" ? optionLines(f.options) : [],
          required: f.required,
          online: f.online,
          panel: f.panel,
          help: f.help.trim(),
        })),
      },
      payment: { ...values.payment },
      policies: { ...values.policies },
      sales_channels: values.sales_channels.map((c) => ({
        code: c.code as OrganizationSettings["sales_channels"][number]["code"],
        is_active: c.is_active,
      })),
      embed: { allowed_origins: originLines(values.embed.allowed_origins) },
      tax: {
        default_vat_rate: values.tax.default_vat_rate,
        invoice_item_mappings: values.tax.invoice_item_mappings.map((m) => ({
          ...m,
          item_type:
            m.item_type as OrganizationSettings["tax"]["invoice_item_mappings"][number]["item_type"],
        })),
      },
    },
  };
}

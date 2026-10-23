import { z } from "zod";

import {
  CODE_PATTERN,
  DAY_KEYS,
  MONEY_PATTERN,
  SERVICE_POINT_TYPES,
  SLUG_PATTERN,
  TIME_PATTERN,
  defaultWeeklyHours,
} from "@/features/branches/constants";
import type {
  Branch,
  BranchCreateRequest,
  DayHours,
  ServicePoint,
  ServicePointCreateRequest,
  WeeklyHours,
} from "@/features/branches/types";

type Translate = (key: string) => string;

const dayHoursSchema = z.object({
  closed: z.boolean(),
  open: z.string(),
  close: z.string(),
});

export const weeklyHoursSchema = z.object({
  mon: dayHoursSchema,
  tue: dayHoursSchema,
  wed: dayHoursSchema,
  thu: dayHoursSchema,
  fri: dayHoursSchema,
  sat: dayHoursSchema,
  sun: dayHoursSchema,
});

function checkDay(
  t: Translate,
  ctx: z.RefinementCtx,
  path: (string | number)[],
  day: DayHours,
) {
  if (day.closed) return;
  const okOpen = TIME_PATTERN.test(day.open);
  const okClose = TIME_PATTERN.test(day.close);
  if (!okOpen) {
    ctx.addIssue({
      code: "custom",
      path: [...path, "open"],
      message: t("branches.validation.time"),
    });
  }
  if (!okClose) {
    ctx.addIssue({
      code: "custom",
      path: [...path, "close"],
      message: t("branches.validation.time"),
    });
  }
  if (okOpen && okClose && day.open >= day.close) {
    ctx.addIssue({
      code: "custom",
      path: [...path, "close"],
      message: t("branches.validation.close_after_open"),
    });
  }
}

export function checkWeek(
  t: Translate,
  ctx: z.RefinementCtx,
  path: (string | number)[],
  hours: WeeklyHours,
) {
  for (const key of DAY_KEYS) {
    checkDay(t, ctx, [...path, key], hours[key]);
  }
}

function optionalNumber(
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
      const n = Number(v.replace(",", "."));
      return Number.isFinite(n) && n >= min && n <= max;
    }, t(messageKey));
}

export function createBranchFormSchema(t: Translate) {
  return z
    .object({
      code: z
        .string()
        .trim()
        .min(1, t("branches.validation.code_required"))
        .regex(CODE_PATTERN, t("branches.validation.code_format")),
      name: z
        .string()
        .trim()
        .min(1, t("branches.validation.name_required"))
        .max(255),
      address: z.string().max(1000),
      city: z.string().max(128),
      district: z.string().max(128),
      phone: z.string().max(32),
      email: z
        .string()
        .trim()
        .refine(
          (v) => v === "" || z.email().safeParse(v).success,
          t("branches.validation.email"),
        ),
      latitude: optionalNumber(t, -90, 90, "branches.validation.latitude"),
      longitude: optionalNumber(t, -180, 180, "branches.validation.longitude"),
      prep_minutes: z
        .string()
        .trim()
        .refine((v) => {
          const n = Number(v);
          return v !== "" && Number.isInteger(n) && n >= 0 && n <= 10080;
        }, t("branches.validation.prep_minutes")),
      is_active: z.boolean(),
      slug: z
        .string()
        .trim()
        .max(64)
        .refine(
          (v) => v === "" || SLUG_PATTERN.test(v),
          t("branches.validation.slug"),
        ),
      description: z.string().max(2000),
      /** "" follows the business time zone. */
      timezone: z.string(),
      show_online: z.boolean(),
      working_hours: weeklyHoursSchema,
      holiday_exceptions: z.array(
        z.object({
          date: z.string(),
          closed: z.boolean(),
          open: z.string(),
          close: z.string(),
          note: z.string().max(255),
        }),
      ),
    })
    .superRefine((values, ctx) => {
      if ((values.latitude === "") !== (values.longitude === "")) {
        ctx.addIssue({
          code: "custom",
          path: [values.latitude === "" ? "latitude" : "longitude"],
          message: t("branches.validation.coordinates_pair"),
        });
      }
      checkWeek(t, ctx, ["working_hours"], values.working_hours);
      const seen = new Set<string>();
      values.holiday_exceptions.forEach((holiday, index) => {
        const path = ["holiday_exceptions", index];
        if (!/^\d{4}-\d{2}-\d{2}$/.test(holiday.date)) {
          ctx.addIssue({
            code: "custom",
            path: [...path, "date"],
            message: t("branches.validation.date"),
          });
        } else if (seen.has(holiday.date)) {
          ctx.addIssue({
            code: "custom",
            path: [...path, "date"],
            message: t("branches.validation.date_duplicate"),
          });
        }
        seen.add(holiday.date);
        checkDay(t, ctx, path, holiday);
      });
    });
}

export type BranchFormValues = z.infer<
  ReturnType<typeof createBranchFormSchema>
>;

export function branchFormDefaults(branch?: Branch): BranchFormValues {
  return {
    code: branch?.code ?? "",
    name: branch?.name ?? "",
    address: branch?.address ?? "",
    city: branch?.city ?? "",
    district: branch?.district ?? "",
    phone: branch?.phone ?? "",
    email: branch?.email ?? "",
    latitude: branch?.latitude != null ? String(branch.latitude) : "",
    longitude: branch?.longitude != null ? String(branch.longitude) : "",
    prep_minutes: String(branch?.prep_minutes ?? 60),
    is_active: branch?.is_active ?? true,
    slug: branch?.slug ?? "",
    description: branch?.description ?? "",
    timezone: branch?.timezone ?? "",
    show_online: branch?.show_online ?? true,
    working_hours: branch?.working_hours ?? defaultWeeklyHours(),
    holiday_exceptions: (branch?.holiday_exceptions ?? []).map((h) => ({
      ...h,
    })),
  };
}

function numberOrNull(value: string): number | null {
  const v = value.trim();
  if (!v) return null;
  return Number(v.replace(",", "."));
}

function normalizeDay(day: DayHours): DayHours {
  return day.closed ? { closed: true, open: "", close: "" } : day;
}

export function normalizeWeek(hours: WeeklyHours): WeeklyHours {
  const out = { ...hours };
  for (const key of DAY_KEYS) out[key] = normalizeDay(hours[key]);
  return out;
}

/** Form values → API body (create and full update share the shape). */
export function branchFormToRequest(
  values: BranchFormValues,
): BranchCreateRequest {
  return {
    code: values.code.trim().toUpperCase(),
    name: values.name.trim(),
    address: values.address.trim(),
    city: values.city.trim(),
    district: values.district.trim(),
    phone: values.phone.trim(),
    email: values.email.trim(),
    latitude: numberOrNull(values.latitude),
    longitude: numberOrNull(values.longitude),
    prep_minutes: Number(values.prep_minutes),
    is_active: values.is_active,
    slug: values.slug.trim().toLowerCase(),
    description: values.description.trim(),
    timezone: values.timezone || null,
    show_online: values.show_online,
    working_hours: normalizeWeek(values.working_hours),
    holiday_exceptions: values.holiday_exceptions.map((h) => ({
      ...normalizeDay(h),
      date: h.date,
      note: h.note.trim(),
    })),
  };
}

// ------------------------------------------------------------ quick edit

export function createQuickEditSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("branches.validation.name_required"))
      .max(255),
    phone: z.string().max(32),
    email: z
      .string()
      .trim()
      .refine(
        (v) => v === "" || z.email().safeParse(v).success,
        t("branches.validation.email"),
      ),
    prep_minutes: z
      .string()
      .trim()
      .refine((v) => {
        const n = Number(v);
        return v !== "" && Number.isInteger(n) && n >= 0 && n <= 10080;
      }, t("branches.validation.prep_minutes")),
    is_active: z.boolean(),
  });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

// ---------------------------------------------------------- service point

export function createServicePointSchema(t: Translate) {
  return z
    .object({
      type: z.enum(SERVICE_POINT_TYPES as [string, ...string[]]),
      name: z
        .string()
        .trim()
        .min(1, t("branches.validation.name_required"))
        .max(255),
      description: z.string().max(2000),
      extra_fee: z
        .string()
        .trim()
        .regex(MONEY_PATTERN, t("branches.validation.extra_fee")),
      online_selectable: z.boolean(),
      is_active: z.boolean(),
      own_hours: z.boolean(),
      working_hours: weeklyHoursSchema,
    })
    .superRefine((values, ctx) => {
      if (values.own_hours) {
        checkWeek(t, ctx, ["working_hours"], values.working_hours);
      }
    });
}

export type ServicePointFormValues = z.infer<
  ReturnType<typeof createServicePointSchema>
>;

export function servicePointFormDefaults(
  point?: ServicePoint,
  branchHours?: WeeklyHours,
): ServicePointFormValues {
  return {
    type: point?.type ?? "office",
    name: point?.name ?? "",
    description: point?.description ?? "",
    extra_fee: point?.extra_fee ?? "0.00",
    online_selectable: point?.online_selectable ?? true,
    is_active: point?.is_active ?? true,
    own_hours: Boolean(point?.working_hours),
    working_hours: point?.working_hours ?? branchHours ?? defaultWeeklyHours(),
  };
}

export function servicePointFormToRequest(
  values: ServicePointFormValues,
): ServicePointCreateRequest {
  return {
    type: values.type as ServicePointCreateRequest["type"],
    name: values.name.trim(),
    description: values.description.trim(),
    extra_fee: values.extra_fee.trim().replace(",", "."),
    online_selectable: values.online_selectable,
    is_active: values.is_active,
    working_hours: values.own_hours
      ? normalizeWeek(values.working_hours)
      : null,
  };
}

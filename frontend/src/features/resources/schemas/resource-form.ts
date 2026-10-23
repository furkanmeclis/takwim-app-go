import { z } from "zod";

import { defaultWeeklyHours } from "@/features/branches/constants";
import {
  checkWeek,
  normalizeWeek,
  weeklyHoursSchema,
} from "@/features/branches/schemas/branch-form";
import type { WeeklyHours } from "@/features/branches/types";
import {
  MAX_CAPACITY,
  MAX_SORT_ORDER,
  RESOURCE_KINDS,
} from "@/features/resources/constants";
import type {
  Closure,
  ClosureCreateRequest,
  Resource,
  ResourceCreateRequest,
  ResourceKind,
} from "@/features/resources/types";
import { isoToLocalInput, localInputToISO } from "@/lib/datetime/local-input";

type Translate = (key: string) => string;

function intField(t: Translate, min: number, max: number, messageKey: string) {
  return z
    .string()
    .trim()
    .refine((v) => {
      const n = Number(v);
      return v !== "" && Number.isInteger(n) && n >= min && n <= max;
    }, t(messageKey));
}

export function createResourceFormSchema(t: Translate) {
  return z
    .object({
      branch_uuid: z.string().min(1, t("resources.validation.branch_required")),
      kind: z.enum(RESOURCE_KINDS as [string, ...string[]]),
      name: z
        .string()
        .trim()
        .min(1, t("resources.validation.name_required"))
        .max(120, t("resources.validation.name_max")),
      zone: z.string().trim().max(64, t("resources.validation.zone_max")),
      description: z.string().max(2000),
      min_capacity: intField(
        t,
        1,
        MAX_CAPACITY,
        "resources.validation.capacity",
      ),
      capacity: intField(t, 1, MAX_CAPACITY, "resources.validation.capacity"),
      combinable: z.boolean(),
      sort_order: intField(
        t,
        0,
        MAX_SORT_ORDER,
        "resources.validation.sort_order",
      ),
      show_online: z.boolean(),
      is_active: z.boolean(),
      own_hours: z.boolean(),
      working_hours: weeklyHoursSchema,
    })
    .superRefine((values, ctx) => {
      if (Number(values.capacity) < Number(values.min_capacity)) {
        ctx.addIssue({
          code: "custom",
          path: ["capacity"],
          message: t("resources.validation.capacity_below_min"),
        });
      }
      if (values.own_hours) {
        checkWeek(t, ctx, ["working_hours"], values.working_hours);
      }
    });
}

export type ResourceFormValues = z.infer<
  ReturnType<typeof createResourceFormSchema>
>;

export function resourceFormDefaults(
  resource?: Resource,
  defaults?: { branchUuid?: string; kind?: ResourceKind },
): ResourceFormValues {
  const hours: WeeklyHours | undefined =
    resource?.working_hours ?? resource?.branch_hours ?? undefined;
  return {
    branch_uuid: resource?.branch.uuid ?? defaults?.branchUuid ?? "",
    kind: resource?.kind ?? defaults?.kind ?? "chair",
    name: resource?.name ?? "",
    zone: resource?.zone ?? "",
    description: resource?.description ?? "",
    min_capacity: String(resource?.min_capacity ?? 1),
    capacity: String(resource?.capacity ?? 1),
    combinable: resource?.combinable ?? false,
    sort_order: String(resource?.sort_order ?? 0),
    show_online: resource?.show_online ?? true,
    is_active: resource?.is_active ?? true,
    own_hours: Boolean(resource?.working_hours),
    working_hours: hours ?? defaultWeeklyHours(),
  };
}

export function resourceFormToRequest(
  values: ResourceFormValues,
): ResourceCreateRequest {
  return {
    branch_uuid: values.branch_uuid,
    kind: values.kind as ResourceKind,
    name: values.name.trim(),
    zone: values.zone.trim(),
    description: values.description.trim(),
    min_capacity: Number(values.min_capacity),
    capacity: Number(values.capacity),
    // Only tables are joined; the flag means nothing for other kinds.
    combinable: values.kind === "table" ? values.combinable : false,
    sort_order: Number(values.sort_order),
    show_online: values.show_online,
    is_active: values.is_active,
    working_hours: values.own_hours
      ? normalizeWeek(values.working_hours)
      : null,
  };
}

export function createQuickEditSchema(t: Translate) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t("resources.validation.name_required"))
        .max(120, t("resources.validation.name_max")),
      zone: z.string().trim().max(64, t("resources.validation.zone_max")),
      min_capacity: intField(
        t,
        1,
        MAX_CAPACITY,
        "resources.validation.capacity",
      ),
      capacity: intField(t, 1, MAX_CAPACITY, "resources.validation.capacity"),
      show_online: z.boolean(),
      is_active: z.boolean(),
    })
    .superRefine((values, ctx) => {
      if (Number(values.capacity) < Number(values.min_capacity)) {
        ctx.addIssue({
          code: "custom",
          path: ["capacity"],
          message: t("resources.validation.capacity_below_min"),
        });
      }
    });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

export function createClosureSchema(t: Translate) {
  return z
    .object({
      starts_at: z.string().min(1, t("resources.validation.starts_required")),
      ends_at: z.string().min(1, t("resources.validation.ends_required")),
      reason: z.string().max(1000),
    })
    .superRefine((values, ctx) => {
      if (
        values.starts_at &&
        values.ends_at &&
        values.ends_at <= values.starts_at
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["ends_at"],
          message: t("resources.validation.ends_after_starts"),
        });
      }
    });
}

export type ClosureFormValues = z.infer<ReturnType<typeof createClosureSchema>>;

export function closureFormDefaults(closure?: Closure): ClosureFormValues {
  if (closure) {
    return {
      starts_at: isoToLocalInput(closure.starts_at),
      ends_at: isoToLocalInput(closure.ends_at),
      reason: closure.reason,
    };
  }
  const start = new Date();
  start.setHours(9, 0, 0, 0);
  const end = new Date(start);
  end.setHours(18, 0, 0, 0);
  return {
    starts_at: isoToLocalInput(start.toISOString()),
    ends_at: isoToLocalInput(end.toISOString()),
    reason: "",
  };
}

export function closureFormToRequest(
  values: ClosureFormValues,
): ClosureCreateRequest {
  return {
    starts_at: localInputToISO(values.starts_at) ?? values.starts_at,
    ends_at: localInputToISO(values.ends_at) ?? values.ends_at,
    reason: values.reason.trim(),
  };
}

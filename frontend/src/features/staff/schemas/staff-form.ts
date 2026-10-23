import { z } from "zod";

import {
  COLOR_PATTERN,
  DEFAULT_STAFF_COLOR,
  TIME_OFF_KINDS,
} from "@/features/staff/constants";
import type {
  Staff,
  StaffCreateRequest,
  TimeOff,
  TimeOffCreateRequest,
} from "@/features/staff/types";
import { isoToLocalInput, localInputToISO } from "@/lib/datetime/local-input";

type Translate = (key: string) => string;

/** "No account" choice of the account select (selects need a value). */
export const NO_ACCOUNT = "none";

function emailField(t: Translate) {
  return z
    .string()
    .trim()
    .refine(
      (v) => v === "" || z.email().safeParse(v).success,
      t("staff.validation.email"),
    );
}

export function createStaffFormSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("staff.validation.name_required"))
      .max(120, t("staff.validation.name_max")),
    title: z.string().trim().max(120),
    bio: z.string().max(2000),
    phone: z.string().trim().max(32),
    email: emailField(t),
    color: z.string().regex(COLOR_PATTERN, t("staff.validation.color")),
    account_uuid: z.string(),
    branch_uuids: z.array(z.string()),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type StaffFormValues = z.infer<ReturnType<typeof createStaffFormSchema>>;

export function staffFormDefaults(staff?: Staff): StaffFormValues {
  return {
    name: staff?.name ?? "",
    title: staff?.title ?? "",
    bio: staff?.bio ?? "",
    phone: staff?.phone ?? "",
    email: staff?.email ?? "",
    color: staff?.color ?? DEFAULT_STAFF_COLOR,
    account_uuid: staff?.account?.uuid ?? NO_ACCOUNT,
    // Only branches the caller may manage: the API keeps the others.
    branch_uuids: staff?.assignments
      ? staff.assignments.filter((a) => a.editable).map((a) => a.branch_uuid)
      : (staff?.branches.map((branch) => branch.uuid) ?? []),
    show_online: staff?.show_online ?? true,
    is_active: staff?.is_active ?? true,
  };
}

export function staffFormToRequest(
  values: StaffFormValues,
): StaffCreateRequest {
  return {
    name: values.name.trim(),
    title: values.title.trim(),
    bio: values.bio,
    phone: values.phone.trim(),
    email: values.email.trim(),
    color: values.color.toUpperCase(),
    account_uuid:
      values.account_uuid === NO_ACCOUNT ? null : values.account_uuid,
    branch_uuids: values.branch_uuids,
    show_online: values.show_online,
    is_active: values.is_active,
  };
}

export function createQuickEditSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("staff.validation.name_required"))
      .max(120, t("staff.validation.name_max")),
    title: z.string().trim().max(120),
    phone: z.string().trim().max(32),
    email: emailField(t),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

export function createTimeOffSchema(t: Translate) {
  return z
    .object({
      kind: z.enum(TIME_OFF_KINDS as [string, ...string[]]),
      starts_at: z.string().min(1, t("staff.validation.starts_required")),
      ends_at: z.string().min(1, t("staff.validation.ends_required")),
      note: z.string().max(1000),
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
          message: t("staff.validation.ends_after_starts"),
        });
      }
    });
}

export type TimeOffFormValues = z.infer<ReturnType<typeof createTimeOffSchema>>;

export function timeOffFormDefaults(timeOff?: TimeOff): TimeOffFormValues {
  if (timeOff) {
    return {
      kind: timeOff.kind,
      starts_at: isoToLocalInput(timeOff.starts_at),
      ends_at: isoToLocalInput(timeOff.ends_at),
      note: timeOff.note,
    };
  }
  const start = new Date();
  start.setHours(9, 0, 0, 0);
  const end = new Date(start);
  end.setHours(18, 0, 0, 0);
  return {
    kind: "leave",
    starts_at: isoToLocalInput(start.toISOString()),
    ends_at: isoToLocalInput(end.toISOString()),
    note: "",
  };
}

export function timeOffFormToRequest(
  values: TimeOffFormValues,
): TimeOffCreateRequest {
  return {
    kind: values.kind as TimeOffCreateRequest["kind"],
    starts_at: localInputToISO(values.starts_at) ?? values.starts_at,
    ends_at: localInputToISO(values.ends_at) ?? values.ends_at,
    note: values.note.trim(),
  };
}

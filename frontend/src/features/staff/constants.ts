import type { ShiftDayKey, TimeOffKind } from "@/features/staff/types";

/** Resource slug shared by io / bulk / activity. */
export const STAFF_RESOURCE = "tenant.staff" as const;

export const DEFAULT_STAFF_COLOR = "#2563EB";

/** Calendar colors offered in the form (any `#RRGGBB` is accepted). */
export const STAFF_COLORS = [
  "#2563EB",
  "#7C3AED",
  "#DB2777",
  "#DC2626",
  "#EA580C",
  "#CA8A04",
  "#16A34A",
  "#0D9488",
  "#0891B2",
  "#475569",
];

export const COLOR_PATTERN = /^#[0-9A-Fa-f]{6}$/;

export const TIME_OFF_KINDS: TimeOffKind[] = [
  "leave",
  "sick",
  "training",
  "other",
];

/** i18n keys per time off kind (static so the i18n checker can verify them). */
export const TIME_OFF_KIND_LABEL_KEYS: Record<TimeOffKind, string> = {
  leave: "staff.time_off.kinds.leave",
  sick: "staff.time_off.kinds.sick",
  training: "staff.time_off.kinds.training",
  other: "staff.time_off.kinds.other",
};

export const SHIFT_DAY_KEYS: ShiftDayKey[] = [
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
  "sun",
];

/** API limits of the shift editor (see PUT …/shifts). */
export const MAX_SHIFTS_PER_DAY = 6;
export const MAX_BREAKS_PER_SHIFT = 4;

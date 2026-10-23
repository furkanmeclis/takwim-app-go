import { SHIFT_DAY_KEYS } from "@/features/staff/constants";
import type { Shift, WeeklyShifts } from "@/features/staff/types";

export function emptyWeek(): WeeklyShifts {
  return { mon: [], tue: [], wed: [], thu: [], fri: [], sat: [], sun: [] };
}

/** Deep copy so the editor never mutates the cached record. */
export function cloneWeek(week: WeeklyShifts): WeeklyShifts {
  const out = emptyWeek();
  for (const day of SHIFT_DAY_KEYS) {
    out[day] = (week[day] ?? []).map((shift) => ({
      start: shift.start,
      end: shift.end,
      breaks: (shift.breaks ?? []).map((b) => ({ ...b })),
    }));
  }
  return out;
}

/** Shifts and their breaks in start order (the API answers with indexes). */
export function sortWeek(week: WeeklyShifts): WeeklyShifts {
  const out = cloneWeek(week);
  for (const day of SHIFT_DAY_KEYS) {
    out[day].sort((a, b) => a.start.localeCompare(b.start));
    for (const shift of out[day]) {
      shift.breaks.sort((a, b) => a.start.localeCompare(b.start));
    }
  }
  return out;
}

function minutes(value: string) {
  const [h, m] = value.split(":").map(Number);
  return (h ?? 0) * 60 + (m ?? 0);
}

/** Working minutes of a shift (breaks excluded). */
export function shiftMinutes(shift: Shift) {
  const span = minutes(shift.end) - minutes(shift.start);
  const pauses = shift.breaks.reduce(
    (sum, b) => sum + Math.max(0, minutes(b.end) - minutes(b.start)),
    0,
  );
  return Math.max(0, span - pauses);
}

export function weekMinutes(week: WeeklyShifts) {
  return SHIFT_DAY_KEYS.reduce(
    (sum, day) =>
      sum + (week[day] ?? []).reduce((acc, s) => acc + shiftMinutes(s), 0),
    0,
  );
}

/** "37,5" style hours for a minute count. */
export function hoursLabel(totalMinutes: number, locale: string) {
  return (totalMinutes / 60).toLocaleString(locale, {
    maximumFractionDigits: 1,
  });
}

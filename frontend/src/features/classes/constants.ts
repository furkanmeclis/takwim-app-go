import type { ClassSessionState, Weekday } from "@/features/classes/types";

/** Engine / live slug (activity, realtime). */
export const CLASSES_RESOURCE = "tenant.classes" as const;

/** Sector module that turns the timetable on. */
export const CLASSES_MODULE = "classes" as const;

export const WEEKDAYS: Weekday[] = [
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
  "sun",
];

export const WEEKDAY_LABEL_KEYS: Record<Weekday, string> = {
  mon: "classes.weekdays.mon",
  tue: "classes.weekdays.tue",
  wed: "classes.weekdays.wed",
  thu: "classes.weekdays.thu",
  fri: "classes.weekdays.fri",
  sat: "classes.weekdays.sat",
  sun: "classes.weekdays.sun",
};

export const SESSION_STATES: ClassSessionState[] = [
  "open",
  "full",
  "cancelled",
  "past",
];

export const SESSION_STATE_LABEL_KEYS: Record<ClassSessionState, string> = {
  open: "classes.session_states.open",
  full: "classes.session_states.full",
  cancelled: "classes.session_states.cancelled",
  past: "classes.session_states.past",
};

export const SESSION_STATE_TONES = {
  open: "success",
  full: "warning",
  cancelled: "danger",
  past: "default",
} as const satisfies Record<
  ClassSessionState,
  "default" | "success" | "warning" | "danger"
>;

/** API limits (see `ClassScheduleInput`, `ClassSessionChangeInput`). */
export const MIN_CAPACITY = 1;
export const MAX_CAPACITY = 1000;
export const MAX_NOTE = 500;
export const MAX_REASON = 500;
/** The sessions list covers at most this many days. */
export const MAX_SESSION_DAYS = 31;

import type {
  DayKey,
  ServicePointType,
  WeeklyHours,
} from "@/features/branches/types";

/** Resource slug shared by io / bulk / activity. */
export const BRANCHES_RESOURCE = "tenant.branches" as const;

export const DAY_KEYS: DayKey[] = [
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
  "sun",
];

/** i18n keys per weekday (static so the i18n checker can verify them). */
export const DAY_LABEL_KEYS: Record<DayKey, string> = {
  mon: "branches.days.mon",
  tue: "branches.days.tue",
  wed: "branches.days.wed",
  thu: "branches.days.thu",
  fri: "branches.days.fri",
  sat: "branches.days.sat",
  sun: "branches.days.sun",
};

export const SERVICE_POINT_TYPE_LABEL_KEYS: Record<ServicePointType, string> = {
  office: "branches.service_points.types.office",
  airport: "branches.service_points.types.airport",
  address_zone: "branches.service_points.types.address_zone",
  hotel: "branches.service_points.types.hotel",
};

export const SERVICE_POINT_TYPES: ServicePointType[] = [
  "office",
  "airport",
  "address_zone",
  "hotel",
];

/** Same default the API applies when a branch is created without hours. */
export function defaultWeeklyHours(): WeeklyHours {
  const day = { closed: false, open: "09:00", close: "18:00" };
  return {
    mon: { ...day },
    tue: { ...day },
    wed: { ...day },
    thu: { ...day },
    fri: { ...day },
    sat: { closed: false, open: "09:00", close: "14:00" },
    sun: { closed: true, open: "", close: "" },
  };
}

export const TIME_PATTERN = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;
export const CODE_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$/;
export const SLUG_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/;
export const MONEY_PATTERN = /^[0-9]{1,12}([.,][0-9]{1,2})?$/;

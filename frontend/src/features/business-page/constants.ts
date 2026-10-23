/**
 * The shared customer panel (TEC-909): takwim.link/panel. "Randevularım"
 * on the business page opens it (requirements §14A).
 */
export const CUSTOMER_PANEL_PATH = "/panel";

/** Weekday keys of `working_hours`, Monday first. */
export const WEEKDAYS = [
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
  "sun",
] as const;

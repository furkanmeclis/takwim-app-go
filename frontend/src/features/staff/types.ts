import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Staff = Schemas["Staff"];
export type StaffPage = Schemas["StaffPage"];
export type StaffFacets = Schemas["StaffFacets"];
export type StaffCreateRequest = Schemas["StaffCreateRequest"];
export type StaffPatchRequest = Schemas["StaffPatchRequest"];
export type StaffAccount = Schemas["StaffAccount"];
export type StaffAssignment = Schemas["StaffAssignment"];
export type WeeklyShifts = Schemas["StaffWeeklyShifts"];
export type Shift = Schemas["StaffShift"];
export type ShiftBreak = Schemas["StaffBreak"];
export type TimeOff = Schemas["StaffTimeOff"];
export type TimeOffPage = Schemas["StaffTimeOffPage"];
export type TimeOffCreateRequest = Schemas["StaffTimeOffCreateRequest"];
export type TimeOffPatchRequest = Schemas["StaffTimeOffPatchRequest"];
export type StaffActivityPage = Schemas["BranchActivityPage"];
export type StaffActivityEntry = Schemas["BranchActivityEntry"];

export type TimeOffKind = TimeOff["kind"];
export type ShiftDayKey = keyof WeeklyShifts;

/** List query of `GET /v1/tenant/staff`. */
export type ListStaffParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids (faceted filter). */
  branch?: string;
  is_active?: "true" | "false";
  show_online?: "true" | "false";
  has_account?: "true" | "false";
};

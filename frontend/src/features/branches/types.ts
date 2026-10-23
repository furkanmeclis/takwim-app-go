import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Branch = Schemas["Branch"];
export type BranchPage = Schemas["BranchPage"];
export type BranchFacets = Schemas["BranchFacets"];
export type BranchCreateRequest = Schemas["BranchCreateRequest"];
export type BranchPatchRequest = Schemas["BranchPatchRequest"];
export type WeeklyHours = Schemas["WeeklyHours"];
export type DayHours = Schemas["DayHours"];
export type HolidayException = Schemas["HolidayException"];
export type ServicePoint = Schemas["ServicePoint"];
export type ServicePointCreateRequest = Schemas["ServicePointCreateRequest"];
export type ServicePointPatchRequest = Schemas["ServicePointPatchRequest"];
export type BranchActivityEntry = Schemas["BranchActivityEntry"];
export type BranchActivityPage = Schemas["BranchActivityPage"];

export type ServicePointType = ServicePoint["type"];

export type DayKey = keyof WeeklyHours;

/** List query of `GET /v1/tenant/branches`. */
export type ListBranchesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated cities (faceted filter). */
  city?: string;
  is_active?: "true" | "false";
};

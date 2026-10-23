import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type OrganizationSummary = Schemas["OrganizationSummary"];
export type OrganizationSummaryPage = Schemas["OrganizationSummaryPage"];
export type Organization = Schemas["Organization"];
export type CreateOrganizationRequest = Schemas["CreateOrganizationRequest"];
export type CreateOrganizationResult = Schemas["CreateOrganizationResult"];
export type PatchOrganizationRequest = Schemas["PatchOrganizationRequest"];
export type OrganizationActivityEntry = Schemas["OrganizationActivityEntry"];
export type OrganizationActivityPage = Schemas["OrganizationActivityPage"];
export type PlatformMemberRequest = Schemas["PlatformMemberRequest"];
export type PlatformMemberPatchRequest = Schemas["PlatformMemberPatchRequest"];
export type BranchOption = Schemas["BranchOption"];
export type OrganizationStatus = OrganizationSummary["status"];
export type OrganizationAccessState = OrganizationSummary["access_state"];

/** List query of `GET /v1/platform/organizations` (multi values comma-joined). */
export type ListOrganizationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  access?: string;
  window?: string;
};

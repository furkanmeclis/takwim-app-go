import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type TenantMember = Schemas["TenantMember"];
export type TenantMemberPage = Schemas["TenantMemberPage"];
export type TenantMemberPatchRequest = Schemas["TenantMemberPatchRequest"];
export type BranchScope = TenantMember["branch_scope"];

/** List query of `GET /v1/tenant/members`. */
export type ListTenantMembersParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated role uuids (any of them). */
  role?: string;
  branch_scope?: BranchScope;
};

/** Engine slug (live list channel `org:{uuid}:resource:tenant.members`). */
export const TENANT_MEMBERS_RESOURCE = "tenant.members" as const;

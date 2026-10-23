import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Resource = Schemas["TenantResource"];
export type ResourcePage = Schemas["TenantResourcePage"];
export type ResourceFacets = Schemas["TenantResourceFacets"];
export type ResourceCreateRequest = Schemas["TenantResourceCreateRequest"];
export type ResourcePatchRequest = Schemas["TenantResourcePatchRequest"];
export type Closure = Schemas["TenantResourceClosure"];
export type ClosurePage = Schemas["TenantResourceClosurePage"];
export type ClosureCreateRequest =
  Schemas["TenantResourceClosureCreateRequest"];
export type ClosurePatchRequest = Schemas["TenantResourceClosurePatchRequest"];
export type ResourceActivityPage = Schemas["BranchActivityPage"];
export type ResourceActivityEntry = Schemas["BranchActivityEntry"];

export type ResourceKind = Resource["kind"];

/** List query of `GET /v1/tenant/resources`. */
export type ListResourcesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids (faceted filter). */
  branch?: string;
  /** Comma-separated kinds. */
  kind?: string;
  /** Comma-separated zones. */
  zone?: string;
  is_active?: "true" | "false";
  show_online?: "true" | "false";
};

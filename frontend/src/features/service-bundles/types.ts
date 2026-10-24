import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ServiceBundle = Schemas["TenantServiceBundle"];
export type ServiceBundleItem = Schemas["TenantServiceBundleItem"];
export type ServiceBundlePage = Schemas["TenantServiceBundlePage"];
export type ServiceBundleFacets = Schemas["TenantServiceBundleFacets"];
export type ServiceBundleCreateRequest =
  Schemas["TenantServiceBundleCreateRequest"];
export type ServiceBundlePatchRequest =
  Schemas["TenantServiceBundlePatchRequest"];
export type ServiceBundleActivityPage = Schemas["BranchActivityPage"];

/** List query of `GET /v1/tenant/service-bundles`. */
export type ListServiceBundlesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids (bundles offered there). */
  branch?: string;
  is_active?: "true" | "false";
  show_online?: "true" | "false";
};

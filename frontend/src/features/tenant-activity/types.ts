import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type TenantActivityEvent = Schemas["TenantActivityEvent"];
export type TenantActivityPage = Schemas["TenantActivityPage"];
export type TenantActivityFacets = Schemas["TenantActivityFacets"];

export type ListTenantActivityParams = {
  limit?: number;
  offset?: number;
  q?: string;
  sort?: string;
  /** Comma-separated resources. */
  resource?: string;
  /** Comma-separated actions. */
  action?: string;
  resource_uuid?: string;
  actor_uuid?: string;
  from?: string;
  to?: string;
};

/** One changed field: `payload.changes[path]`. */
export type ActivityChange = { before: unknown; after: unknown };

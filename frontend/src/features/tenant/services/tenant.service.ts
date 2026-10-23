import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TenantContextData = components["schemas"]["TenantContext"];
export type TenantOverview = components["schemas"]["TenantOverview"];
export type TenantOverviewBranch =
  components["schemas"]["TenantOverviewBranch"];

export const tenantKeys = {
  all: ["tenant"] as const,
  context: (slug: string) => ["tenant", slug, "context"] as const,
  overview: (slug: string) => ["tenant", slug, "overview"] as const,
};

export const tenantService = {
  /** Organization scope of `slug` for the signed-in user. */
  getContext(slug: string, signal?: AbortSignal) {
    return tenantRequest<TenantContextData>(slug, "GET", "/v1/tenant/context", {
      silent: true,
      signal,
    });
  },

  /**
   * Central panel: the branches the member sees (narrowed by the header
   * branch picker), one by one and in total.
   */
  getOverview(slug: string) {
    return tenantRequest<TenantOverview>(slug, "GET", "/v1/tenant/overview");
  },
};

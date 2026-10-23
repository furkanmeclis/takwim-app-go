import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TenantContextData = components["schemas"]["TenantContext"];

export const tenantKeys = {
  all: ["tenant"] as const,
  context: (slug: string) => ["tenant", slug, "context"] as const,
};

export const tenantService = {
  /** Organization scope of `slug` for the signed-in user. */
  getContext(slug: string, signal?: AbortSignal) {
    return tenantRequest<TenantContextData>(slug, "GET", "/v1/tenant/context", {
      silent: true,
      signal,
    });
  },
};

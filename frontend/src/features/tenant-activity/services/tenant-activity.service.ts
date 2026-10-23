import type {
  ListTenantActivityParams,
  TenantActivityFacets,
  TenantActivityPage,
} from "@/features/tenant-activity/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/activity";

/** Activity history of one organization (`X-Organization`, `X-Branch`). */
export const tenantActivityService = {
  list(slug: string, params: ListTenantActivityParams) {
    return tenantRequest<TenantActivityPage>(slug, "GET", BASE, {
      query: params,
    });
  },

  facets(slug: string) {
    return tenantRequest<TenantActivityFacets>(slug, "GET", `${BASE}/facets`);
  },
};

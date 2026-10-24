import type { ResourceMeta } from "@/features/io/types";
import type {
  ListServiceBundlesParams,
  ServiceBundle,
  ServiceBundleActivityPage,
  ServiceBundleCreateRequest,
  ServiceBundleFacets,
  ServiceBundlePage,
  ServiceBundlePatchRequest,
} from "@/features/service-bundles/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/service-bundles";

/**
 * Combined service bundles of one organization (`X-Organization: {slug}`).
 * Bundles belong to the service catalog and use its permissions.
 */
export const serviceBundlesService = {
  list(slug: string, params: ListServiceBundlesParams) {
    return tenantRequest<ServiceBundlePage>(slug, "GET", BASE, {
      query: params,
    });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<ServiceBundleFacets>(slug, "GET", `${BASE}/facets`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<ServiceBundle>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: ServiceBundleCreateRequest) {
    return tenantRequest<ServiceBundle>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: ServiceBundlePatchRequest) {
    return tenantRequest<ServiceBundle>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<ServiceBundleActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },
};

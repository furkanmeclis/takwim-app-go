import type { ResourceMeta } from "@/features/io/types";
import type {
  Customer,
  CustomerActivityPage,
  CustomerCreateRequest,
  CustomerFacets,
  CustomerPage,
  CustomerPatchRequest,
  ListCustomersParams,
} from "@/features/customers/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/customers";

/**
 * Customers of one organization. Every call names the organization through
 * `tenantRequest` (`X-Organization: {slug}`); the API never returns another
 * business's record, even for the same person.
 */
export const customersService = {
  list(slug: string, params: ListCustomersParams) {
    return tenantRequest<CustomerPage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<CustomerFacets>(slug, "GET", `${BASE}/facets`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Customer>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: CustomerCreateRequest) {
    return tenantRequest<Customer>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: CustomerPatchRequest) {
    return tenantRequest<Customer>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  /** Reopens the panel history this business closed for the person (TEC-991). */
  restoreHistory(slug: string, uuid: string) {
    return tenantRequest<Customer>(
      slug,
      "POST",
      `${BASE}/${uuid}/history/restore`,
    );
  },

  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<CustomerActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },
};

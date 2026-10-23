import type { ResourceMeta } from "@/features/io/types";
import type {
  ListServicesParams,
  Service,
  ServiceActivityPage,
  ServiceCategory,
  ServiceCategoryCreateRequest,
  ServiceCategoryList,
  ServiceCategoryPatchRequest,
  ServiceCreateRequest,
  ServiceFacets,
  ServiceOffersRequest,
  ServicePage,
  ServicePatchRequest,
  ServiceStaffRequest,
} from "@/features/services/types";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/services";
const CATEGORIES = "/v1/tenant/service-categories";

/**
 * Service catalog of one organization. Every call names the organization
 * through `tenantRequest` (`X-Organization: {slug}`); offers are limited to
 * the member's branches by the API.
 */
export const servicesService = {
  list(slug: string, params: ListServicesParams) {
    return tenantRequest<ServicePage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<ServiceFacets>(slug, "GET", `${BASE}/facets`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Service>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: ServiceCreateRequest) {
    return tenantRequest<Service>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: ServicePatchRequest) {
    return tenantRequest<Service>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  setOffers(slug: string, uuid: string, body: ServiceOffersRequest) {
    return tenantRequest<Service>(slug, "PUT", `${BASE}/${uuid}/offers`, {
      body,
    });
  },

  setStaff(slug: string, uuid: string, body: ServiceStaffRequest) {
    return tenantRequest<Service>(slug, "PUT", `${BASE}/${uuid}/staff`, {
      body,
    });
  },

  uploadImage(slug: string, uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<Service>(
      slug,
      "PUT",
      `${BASE}/${uuid}/image`,
      form,
    );
  },

  removeImage(slug: string, uuid: string) {
    return tenantRequest<Service>(slug, "DELETE", `${BASE}/${uuid}/image`);
  },

  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<ServiceActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },

  categories(slug: string) {
    return tenantRequest<ServiceCategoryList>(slug, "GET", CATEGORIES);
  },

  createCategory(slug: string, body: ServiceCategoryCreateRequest) {
    return tenantRequest<ServiceCategory>(slug, "POST", CATEGORIES, { body });
  },

  updateCategory(
    slug: string,
    uuid: string,
    body: ServiceCategoryPatchRequest,
  ) {
    return tenantRequest<ServiceCategory>(
      slug,
      "PATCH",
      `${CATEGORIES}/${uuid}`,
      { body },
    );
  },

  removeCategory(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${CATEGORIES}/${uuid}`,
    );
  },
};

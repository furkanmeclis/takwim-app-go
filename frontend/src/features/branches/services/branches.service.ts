import type { ResourceMeta } from "@/features/io/types";
import type {
  Branch,
  BranchActivityPage,
  BranchCreateRequest,
  BranchFacets,
  BranchPage,
  BranchPatchRequest,
  ListBranchesParams,
  ServicePoint,
  ServicePointCreateRequest,
  ServicePointPatchRequest,
} from "@/features/branches/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/branches";

/**
 * Branches of one organization. Every call names the organization through
 * `tenantRequest` (`X-Organization: {slug}`); the API scopes all reads and
 * writes to it.
 */
export const branchesService = {
  list(slug: string, params: ListBranchesParams) {
    return tenantRequest<BranchPage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<BranchFacets>(slug, "GET", `${BASE}/facets`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Branch>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: BranchCreateRequest) {
    return tenantRequest<Branch>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: BranchPatchRequest) {
    return tenantRequest<Branch>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<BranchActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },

  listServicePoints(slug: string, uuid: string) {
    return tenantRequest<{ items: ServicePoint[] }>(
      slug,
      "GET",
      `${BASE}/${uuid}/service-points`,
    );
  },

  createServicePoint(
    slug: string,
    uuid: string,
    body: ServicePointCreateRequest,
  ) {
    return tenantRequest<ServicePoint>(
      slug,
      "POST",
      `${BASE}/${uuid}/service-points`,
      { body },
    );
  },

  updateServicePoint(
    slug: string,
    uuid: string,
    pointUuid: string,
    body: ServicePointPatchRequest,
  ) {
    return tenantRequest<ServicePoint>(
      slug,
      "PATCH",
      `${BASE}/${uuid}/service-points/${pointUuid}`,
      { body },
    );
  },

  removeServicePoint(slug: string, uuid: string, pointUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/service-points/${pointUuid}`,
    );
  },
};

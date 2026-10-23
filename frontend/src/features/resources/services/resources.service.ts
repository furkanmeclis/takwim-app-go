import type { ResourceMeta } from "@/features/io/types";
import type {
  Closure,
  ClosureCreateRequest,
  ClosurePage,
  ClosurePatchRequest,
  ListResourcesParams,
  Resource,
  ResourceActivityPage,
  ResourceCreateRequest,
  ResourceFacets,
  ResourcePage,
  ResourcePatchRequest,
} from "@/features/resources/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/resources";

/**
 * Resources of one organization. Every call names the organization through
 * `tenantRequest` (`X-Organization: {slug}`); the API scopes all reads and
 * writes to it and to the member's branches.
 */
export const resourcesService = {
  list(slug: string, params: ListResourcesParams) {
    return tenantRequest<ResourcePage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<ResourceFacets>(slug, "GET", `${BASE}/facets`);
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Resource>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: ResourceCreateRequest) {
    return tenantRequest<Resource>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: ResourcePatchRequest) {
    return tenantRequest<Resource>(slug, "PATCH", `${BASE}/${uuid}`, {
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
    return tenantRequest<ResourceActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },

  listClosures(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<ClosurePage>(slug, "GET", `${BASE}/${uuid}/closures`, {
      query: params,
    });
  },

  createClosure(slug: string, uuid: string, body: ClosureCreateRequest) {
    return tenantRequest<Closure>(slug, "POST", `${BASE}/${uuid}/closures`, {
      body,
    });
  },

  updateClosure(
    slug: string,
    uuid: string,
    closureUuid: string,
    body: ClosurePatchRequest,
  ) {
    return tenantRequest<Closure>(
      slug,
      "PATCH",
      `${BASE}/${uuid}/closures/${closureUuid}`,
      { body },
    );
  },

  removeClosure(slug: string, uuid: string, closureUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/closures/${closureUuid}`,
    );
  },
};

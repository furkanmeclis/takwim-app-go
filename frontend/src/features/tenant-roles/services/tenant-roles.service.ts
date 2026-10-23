import type { ResourceMeta } from "@/features/io/types";
import type {
  ListTenantRolesParams,
  TenantPermission,
  TenantRole,
  TenantRolePage,
  TenantRoleRequest,
} from "@/features/tenant-roles/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/roles";

/** Template and organization roles of one organization (`X-Organization`). */
export const tenantRolesService = {
  list(slug: string, params: ListTenantRolesParams) {
    return tenantRequest<TenantRolePage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  permissions(slug: string) {
    return tenantRequest<{ items: TenantPermission[] }>(
      slug,
      "GET",
      `${BASE}/permissions`,
    );
  },

  get(slug: string, uuid: string) {
    return tenantRequest<TenantRole>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: TenantRoleRequest) {
    return tenantRequest<TenantRole>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: TenantRoleRequest) {
    return tenantRequest<TenantRole>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },
};

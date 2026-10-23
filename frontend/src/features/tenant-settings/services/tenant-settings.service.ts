import type { ResourceMeta } from "@/features/io/types";
import type {
  ListSettingsVersionsParams,
  OrganizationSettingsSaveRequest,
  OrganizationSettingsVersion,
  OrganizationSettingsVersionPage,
} from "@/features/tenant-settings/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/settings";

/** Versioned settings of one organization (`X-Organization`). */
export const tenantSettingsService = {
  current(slug: string) {
    return tenantRequest<OrganizationSettingsVersion>(slug, "GET", BASE);
  },

  /** Stores a new version (the API answers 409 when `base_version` is stale). */
  save(slug: string, body: OrganizationSettingsSaveRequest) {
    return tenantRequest<OrganizationSettingsVersion>(slug, "PUT", BASE, {
      body,
    });
  },

  versions(slug: string, params: ListSettingsVersionsParams) {
    return tenantRequest<OrganizationSettingsVersionPage>(
      slug,
      "GET",
      `${BASE}/versions`,
      { query: params },
    );
  },

  versionsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/versions/meta`);
  },

  version(slug: string, version: number) {
    return tenantRequest<OrganizationSettingsVersion>(
      slug,
      "GET",
      `${BASE}/versions/${version}`,
    );
  },
};

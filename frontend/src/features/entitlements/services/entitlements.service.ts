import type {
  EntitlementSummary,
  PlatformEntitlements,
  SetEntitlementsRequest,
} from "@/features/entitlements/types";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";

const platformPath = (uuid: string) =>
  `/v1/platform/organizations/${uuid}/entitlements`;

/** Plan, add-ons, limits and usage (TEC-883). */
export const entitlementsService = {
  tenant(slug: string) {
    return tenantRequest<EntitlementSummary>(
      slug,
      "GET",
      "/v1/tenant/entitlements",
    );
  },

  platform(uuid: string) {
    return platformRequest<PlatformEntitlements>("GET", platformPath(uuid));
  },

  setPlatform(uuid: string, body: SetEntitlementsRequest) {
    return platformRequest<PlatformEntitlements>("PUT", platformPath(uuid), {
      body,
    });
  },
};

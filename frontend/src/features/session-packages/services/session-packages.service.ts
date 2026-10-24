import type { ResourceMeta } from "@/features/io/types";
import type {
  CustomerPackage,
  CustomerPackageFacets,
  CustomerPackagePage,
  CustomerPackageRefundRequest,
  CustomerPackageSellRequest,
  CustomerPackageSummary,
  ListCustomerPackagesParams,
  ListSessionPackagesParams,
  PackagePeriod,
  SessionPackage,
  SessionPackageInput,
  SessionPackagePage,
} from "@/features/session-packages/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const PACKAGES = "/v1/tenant/session-packages";
const SOLD = "/v1/tenant/customer-packages";

/**
 * Multi-session packages of one organization (TEC-920). A definition is N
 * sessions of a service for a price; selling one takes the full price as a
 * payment (not revenue) and keeps the terms on the customer's package.
 * Sessions are used on visits (reservations service); a refund closes the
 * package.
 */
export const sessionPackagesService = {
  packagesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${PACKAGES}/meta`);
  },

  listPackages(slug: string, params: ListSessionPackagesParams) {
    return tenantRequest<SessionPackagePage>(slug, "GET", PACKAGES, {
      query: params,
    });
  },

  getPackage(slug: string, uuid: string) {
    return tenantRequest<SessionPackage>(slug, "GET", `${PACKAGES}/${uuid}`);
  },

  createPackage(slug: string, body: SessionPackageInput) {
    return tenantRequest<SessionPackage>(slug, "POST", PACKAGES, { body });
  },

  updatePackage(slug: string, uuid: string, body: SessionPackageInput) {
    return tenantRequest<SessionPackage>(slug, "PATCH", `${PACKAGES}/${uuid}`, {
      body,
    });
  },

  deletePackage(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", `${PACKAGES}/${uuid}`);
  },

  soldMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${SOLD}/meta`);
  },

  soldFacets(slug: string) {
    return tenantRequest<CustomerPackageFacets>(slug, "GET", `${SOLD}/facets`);
  },

  summary(slug: string, period: PackagePeriod) {
    return tenantRequest<CustomerPackageSummary>(
      slug,
      "GET",
      `${SOLD}/summary`,
      { query: period },
    );
  },

  listSold(slug: string, params: ListCustomerPackagesParams) {
    return tenantRequest<CustomerPackagePage>(slug, "GET", SOLD, {
      query: params,
    });
  },

  getSold(slug: string, uuid: string) {
    return tenantRequest<CustomerPackage>(slug, "GET", `${SOLD}/${uuid}`);
  },

  sell(slug: string, body: CustomerPackageSellRequest) {
    return tenantRequest<CustomerPackage>(slug, "POST", SOLD, { body });
  },

  refund(slug: string, uuid: string, body: CustomerPackageRefundRequest) {
    return tenantRequest<CustomerPackage>(
      slug,
      "POST",
      `${SOLD}/${uuid}/refund`,
      { body },
    );
  },
};

import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Service = Schemas["TenantService"];
export type ServicePage = Schemas["TenantServicePage"];
export type ServiceFacets = Schemas["TenantServiceFacets"];
export type ServiceOffer = Schemas["TenantServiceOffer"];
export type ServiceStaff = Schemas["TenantServiceStaff"];
export type ServiceCreateRequest = Schemas["TenantServiceCreateRequest"];
export type ServicePatchRequest = Schemas["TenantServicePatchRequest"];
export type ServiceOffersRequest = Schemas["TenantServiceOffersRequest"];
export type ServiceStaffRequest = Schemas["TenantServiceStaffRequest"];
export type ServiceCategory = Schemas["ServiceCategory"];
export type ServiceCategoryList = Schemas["ServiceCategoryList"];
export type ServiceCategoryCreateRequest =
  Schemas["ServiceCategoryCreateRequest"];
export type ServiceCategoryPatchRequest =
  Schemas["ServiceCategoryPatchRequest"];
export type ServiceActivityPage = Schemas["BranchActivityPage"];

export type PaymentMode = Service["payment_mode"];
export type StaffMode = Service["staff_mode"];

/** List query of `GET /v1/tenant/services`. */
export type ListServicesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated category uuids and/or `none`. */
  category?: string;
  /** Comma-separated branch uuids (services offered there). */
  branch?: string;
  /** Comma-separated payment modes. */
  payment_mode?: string;
  is_active?: "true" | "false";
  show_online?: "true" | "false";
};

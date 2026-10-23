import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Customer = Schemas["TenantCustomer"];
export type CustomerPage = Schemas["TenantCustomerPage"];
export type CustomerFacets = Schemas["TenantCustomerFacets"];
export type CustomerCreateRequest = Schemas["TenantCustomerCreateRequest"];
export type CustomerPatchRequest = Schemas["TenantCustomerPatchRequest"];
export type CustomerActivityPage = Schemas["BranchActivityPage"];
export type CustomerActivityEntry = Schemas["BranchActivityEntry"];

export type CustomerSource = Customer["source"];

/** List query of `GET /v1/tenant/customers`. */
export type ListCustomersParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated tags (any of). */
  tag?: string;
  /** Comma-separated sources. */
  source?: string;
  verified?: "true" | "false";
  sms_consent?: "true" | "false";
  email_consent?: "true" | "false";
};

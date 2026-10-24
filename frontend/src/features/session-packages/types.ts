import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SessionPackage = Schemas["SessionPackage"];
export type SessionPackageInput = Schemas["SessionPackageInput"];
export type SessionPackagePage = Schemas["SessionPackagePage"];
export type CustomerPackage = Schemas["CustomerPackage"];
export type CustomerPackagePage = Schemas["CustomerPackagePage"];
export type CustomerPackageSession = Schemas["CustomerPackageSession"];
export type CustomerPackageSellRequest = Schemas["CustomerPackageSellRequest"];
export type CustomerPackageRefundRequest =
  Schemas["CustomerPackageRefundRequest"];
export type CustomerPackageFacets = Schemas["CustomerPackageFacets"];
export type CustomerPackageSummary = Schemas["CustomerPackageSummary"];
export type CustomerPackageFigures = Schemas["CustomerPackageFigures"];
export type ReservationPackageSession =
  Schemas["TenantReservationPackageSession"];
export type ReservationPackageOption =
  Schemas["TenantReservationPackageOption"];

export type CustomerPackageState = CustomerPackage["state"];
export type PackageSessionStatus = CustomerPackageSession["status"];
export type PackageMethod = CustomerPackageSellRequest["method"];

/** Query of `GET /v1/tenant/session-packages`. */
export type ListSessionPackagesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  is_active?: "true" | "false";
  service?: string;
  branch?: string;
};

/** Query of `GET /v1/tenant/customer-packages`. */
export type ListCustomerPackagesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  state?: string;
  package?: string;
  branch?: string;
  customer?: string;
  date_from?: string;
  date_to?: string;
};

/** Range of the package report (dates as "2026-06-01"). */
export type PackagePeriod = {
  date_from?: string;
  date_to?: string;
};

import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type CustomerSignInStart = Schemas["CustomerSignInStart"];
export type CustomerSession = Schemas["CustomerSession"];
export type CustomerProfile = Schemas["CustomerProfile"];
export type CustomerBusiness = Schemas["CustomerBusiness"];
export type CustomerMoney = Schemas["CustomerMoney"];
export type CustomerReservationItem = Schemas["CustomerReservationItem"];
export type CustomerReservation = Schemas["CustomerReservation"];
export type CustomerReservationPage = Schemas["CustomerReservationPage"];
export type CustomerAction = Schemas["CustomerAction"];
export type CustomerAvailability = Schemas["CustomerAvailability"];
export type CustomerPaymentSummary = Schemas["CustomerPaymentSummary"];
export type CustomerDevice = Schemas["CustomerDevice"];
export type CustomerDeviceList = Schemas["CustomerDeviceList"];

export type CustomerPackage = Schemas["CustomerPanelPackage"];
export type CustomerPackageSession = Schemas["CustomerPanelPackageSession"];
export type CustomerPackagePage = Schemas["CustomerPanelPackagePage"];
export type CustomerPackageState = CustomerPackage["state"];

/** Query of `GET /v1/customer/packages`. */
export type CustomerPackageParams = {
  limit: number;
  offset: number;
  business?: string;
  state?: string;
};

export type PublicReservation = Schemas["PublicReservation"];

/**
 * Body of `POST /v1/customer/businesses/{slug}/reservations`: the public
 * booking without contact details (the session names the person).
 */
export type CustomerBookingRequest = Omit<
  Schemas["PublicBookingRequest"],
  "customer"
> & { customer?: Schemas["PublicBookingRequest"]["customer"] };

export type CustomerScope = "upcoming" | "past" | "all";

export type CustomerListParams = {
  limit: number;
  offset: number;
  sort?: string;
  scope?: CustomerScope;
  business?: string;
  status?: string;
};

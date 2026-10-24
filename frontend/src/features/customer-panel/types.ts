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

export type CustomerScope = "upcoming" | "past" | "all";

export type CustomerListParams = {
  limit: number;
  offset: number;
  sort?: string;
  scope?: CustomerScope;
  business?: string;
  status?: string;
};

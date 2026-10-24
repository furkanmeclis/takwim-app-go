import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type OrganizationSettings = Schemas["OrganizationSettings"];
export type OrganizationSettingsVersion =
  Schemas["OrganizationSettingsVersion"];
export type OrganizationSettingsVersionSummary =
  Schemas["OrganizationSettingsVersionSummary"];
export type OrganizationSettingsVersionPage =
  Schemas["OrganizationSettingsVersionPage"];
export type OrganizationSettingsSaveRequest =
  Schemas["OrganizationSettingsSaveRequest"];
export type SalesChannelCode = Schemas["OrganizationSalesChannel"]["code"];
export type BookingFieldType = Schemas["OrganizationBookingField"]["type"];
export type BookingStep =
  Schemas["OrganizationBookingSettings"]["hidden_steps"][number];
export type InvoiceItemType =
  Schemas["OrganizationInvoiceItemMapping"]["item_type"];

/** List query of `GET /v1/tenant/settings/versions`. */
export type ListSettingsVersionsParams = {
  limit: number;
  offset: number;
  sort?: string;
};

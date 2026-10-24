import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type CustomerNotificationSettings =
  Schemas["CustomerNotificationSettings"];
export type CustomerNotificationEvent = Schemas["CustomerNotificationEvent"];
export type CustomerNotificationTemplate =
  Schemas["CustomerNotificationTemplate"];
export type CustomerNotificationChannel =
  Schemas["CustomerNotificationChannel"];
export type CustomerNotificationRule = Schemas["CustomerNotificationRule"];
export type CustomerNotificationSaveInput =
  Schemas["CustomerNotificationSaveInput"];
export type CustomerNotificationPreview =
  Schemas["CustomerNotificationPreview"];
export type CustomerNotificationDelivery =
  Schemas["CustomerNotificationDelivery"];
export type CustomerNotificationDeliveryPage =
  Schemas["CustomerNotificationDeliveryPage"];

export type NotificationChannelName = CustomerNotificationChannel["name"];
export type NotificationEventCode = CustomerNotificationEvent["code"];
export type NotificationDeliveryStatus = CustomerNotificationDelivery["status"];

export type CustomerNotificationPreviewInput = {
  event: string;
  channel: NotificationChannelName;
  subject?: string;
  body: string;
};

export type ListCustomerNotificationDeliveriesParams = {
  limit?: number;
  offset?: number;
  q?: string;
  sort?: string;
  /** Comma-separated channels. */
  channel?: string;
  /** Comma-separated statuses. */
  status?: string;
  /** Comma-separated event codes. */
  event?: string;
  customer_uuid?: string;
  reservation_uuid?: string;
  created_from?: string;
  created_to?: string;
};

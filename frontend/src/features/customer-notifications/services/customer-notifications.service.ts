import type {
  CustomerNotificationDeliveryPage,
  CustomerNotificationEvent,
  CustomerNotificationPreview,
  CustomerNotificationPreviewInput,
  CustomerNotificationSaveInput,
  CustomerNotificationSettings,
  ListCustomerNotificationDeliveriesParams,
} from "@/features/customer-notifications/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/customer-notifications";

/** Customer notification rules, texts and history of one organization. */
export const customerNotificationsService = {
  settings(slug: string) {
    return tenantRequest<CustomerNotificationSettings>(
      slug,
      "GET",
      `${BASE}/settings`,
    );
  },

  save(slug: string, body: CustomerNotificationSaveInput) {
    return tenantRequest<CustomerNotificationEvent>(
      slug,
      "PUT",
      `${BASE}/settings`,
      { body },
    );
  },

  reset(slug: string, event: string) {
    return tenantRequest<CustomerNotificationEvent>(
      slug,
      "POST",
      `${BASE}/settings/reset`,
      { body: { event } },
    );
  },

  preview(slug: string, body: CustomerNotificationPreviewInput) {
    return tenantRequest<CustomerNotificationPreview>(
      slug,
      "POST",
      `${BASE}/preview`,
      { body },
    );
  },

  deliveries(slug: string, params: ListCustomerNotificationDeliveriesParams) {
    return tenantRequest<CustomerNotificationDeliveryPage>(
      slug,
      "GET",
      `${BASE}/deliveries`,
      { query: params },
    );
  },
};

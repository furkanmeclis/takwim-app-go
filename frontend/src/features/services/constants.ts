import type { PaymentMode, StaffMode } from "@/features/services/types";

/** Resource slug shared by io / bulk / activity. */
export const SERVICES_RESOURCE = "tenant.services" as const;

export const PAYMENT_MODES: PaymentMode[] = ["on_site", "online", "both"];
export const STAFF_MODES: StaffMode[] = ["required", "optional", "none"];

/** Category facet value of services without a category. */
export const UNCATEGORIZED = "none";

/** API limits (see `TenantServiceCreateRequest`). */
export const MIN_DURATION = 5;
export const MAX_DURATION = 1440;
export const MAX_BUFFER = 480;
export const MAX_SORT_ORDER = 100000;

/** Seats of a group class session (gym). */
export const MIN_GROUP_CAPACITY = 2;
export const MAX_GROUP_CAPACITY = 1000;
export const MAX_SERVICE_IMAGE_BYTES = 5 * 1024 * 1024;

export const PAYMENT_MODE_LABEL_KEYS: Record<PaymentMode, string> = {
  on_site: "services.payment_mode.on_site",
  online: "services.payment_mode.online",
  both: "services.payment_mode.both",
};

export const STAFF_MODE_LABEL_KEYS: Record<StaffMode, string> = {
  required: "services.staff_mode.required",
  optional: "services.staff_mode.optional",
  none: "services.staff_mode.none",
};

import type { CustomerSource } from "@/features/customers/types";

/** Resource slug shared by io / bulk / activity. */
export const CUSTOMERS_RESOURCE = "tenant.customers" as const;

/** Label keys of the record sources (no dynamic i18n keys). */
export const SOURCE_LABEL_KEYS: Record<CustomerSource, string> = {
  panel: "customers.source.panel",
  import: "customers.source.import",
  online: "customers.source.online",
};

/** Label keys of how a phone was proven. */
export const VERIFICATION_LABEL_KEYS: Record<string, string> = {
  otp: "customers.verification.otp",
  panel_login: "customers.verification.panel_login",
};

/** API limits (see `TenantCustomerPatchRequest`). */
export const MAX_NAME = 80;
export const MAX_NOTES = 5000;
export const MAX_TAGS = 20;
export const MAX_TAG_LENGTH = 32;

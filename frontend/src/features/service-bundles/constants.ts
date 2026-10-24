/** Live list slug of combined service bundles (activity, realtime). */
export const SERVICE_BUNDLES_RESOURCE = "tenant.service_bundles" as const;

/** API limits (see `TenantServiceBundleCreateRequest`). */
export const MIN_BUNDLE_ITEMS = 2;
export const MAX_BUNDLE_ITEMS = 10;
export const MAX_BUNDLE_NAME = 160;
export const MAX_BUNDLE_DESCRIPTION = 2000;
export const MAX_BUNDLE_SORT_ORDER = 100000;

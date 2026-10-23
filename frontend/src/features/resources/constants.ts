import type { ResourceKind } from "@/features/resources/types";

/** Resource slug shared by io / bulk / activity. */
export const RESOURCES_RESOURCE = "tenant.resources" as const;

/** Every kind the API accepts, in display order. */
export const RESOURCE_KINDS: ResourceKind[] = [
  "chair",
  "table",
  "area",
  "room",
  "device",
  "bay",
];

/** API limits (see `TenantResourceCreateRequest`). */
export const MAX_CAPACITY = 1000;
export const MAX_SORT_ORDER = 100000;

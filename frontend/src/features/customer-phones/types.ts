import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** A customer number that changed hands (TEC-910 / TEC-972). */
export type CustomerPhoneRelease = Schemas["CustomerPhoneRelease"];
export type CustomerPhoneReleasePage = Schemas["CustomerPhoneReleasePage"];
export type CustomerPhoneReassignmentInput =
  Schemas["CustomerPhoneReassignmentInput"];

export type CustomerPhoneReleaseReason = CustomerPhoneRelease["reason"];
export type CustomerPhoneReleaseSource = CustomerPhoneRelease["source"];

export const RELEASE_REASONS: CustomerPhoneReleaseReason[] = [
  "number_changed",
  "reassigned",
  "dormant",
];

export const RELEASE_SOURCES: CustomerPhoneReleaseSource[] = [
  "customer",
  "platform",
  "system",
];

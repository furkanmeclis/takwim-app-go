import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type PlanCode = Schemas["PlanCode"];
export type Addon = Schemas["Addon"];
export type Plan = Schemas["Plan"];
export type Entitlements = Schemas["Entitlements"];
export type EntitlementSummary = Schemas["EntitlementSummary"];
export type UsageMetric = EntitlementSummary["usage"][number]["metric"];
export type UsageRow = EntitlementSummary["usage"][number];
export type PlatformEntitlements = Schemas["PlatformEntitlements"];
export type SetEntitlementsRequest = Schemas["SetEntitlementsRequest"];

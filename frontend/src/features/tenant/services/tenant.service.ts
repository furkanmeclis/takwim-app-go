import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TenantContextData = components["schemas"]["TenantContext"];
export type TenantOverview = components["schemas"]["TenantOverview"];
export type TenantOverviewBranch =
  components["schemas"]["TenantOverviewBranch"];
export type DailySummary = components["schemas"]["DailySummary"];
export type DailySummaryMoney = components["schemas"]["DailySummaryMoney"];
export type DailySummaryVisit = components["schemas"]["DailySummaryVisit"];
export type DailySummaryBranch = NonNullable<
  NonNullable<DailySummary["business"]>["branches"]
>[number];

export const tenantKeys = {
  all: ["tenant"] as const,
  context: (slug: string) => ["tenant", slug, "context"] as const,
  overview: (slug: string) => ["tenant", slug, "overview"] as const,
  daily: (slug: string, date: string) =>
    ["tenant", slug, "daily-summary", date] as const,
};

export const tenantService = {
  /** Organization scope of `slug` for the signed-in user. */
  getContext(slug: string, signal?: AbortSignal) {
    return tenantRequest<TenantContextData>(slug, "GET", "/v1/tenant/context", {
      silent: true,
      signal,
    });
  },

  /**
   * Central panel: the branches the member sees (narrowed by the header
   * branch picker), one by one and in total.
   */
  getOverview(slug: string) {
    return tenantRequest<TenantOverview>(slug, "GET", "/v1/tenant/overview");
  },

  /**
   * Daily summary of the home page (TEC-926): the member's own day, the
   * day of the branch scope (narrowed by the header branch picker) and
   * what waits for the business. Sections the member may not see are null.
   */
  getDailySummary(slug: string, date: string) {
    return tenantRequest<DailySummary>(
      slug,
      "GET",
      "/v1/tenant/daily-summary",
      { query: { date } },
    );
  },
};

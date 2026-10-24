import type { ResourceMeta } from "@/features/io/types";
import type {
  ListStaffShareLinesParams,
  ListStaffSharePayoutsParams,
  ListStaffShareRulesParams,
  MyStaffShares,
  StaffShareLinePage,
  StaffSharePayout,
  StaffSharePayoutInput,
  StaffSharePayoutPage,
  StaffSharePeriod,
  StaffShareRule,
  StaffShareRuleInput,
  StaffShareRulePage,
  StaffSharesSummary,
} from "@/features/staff-shares/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/staff-shares";
const RULES = `${BASE}/rules`;
const LINES = `${BASE}/lines`;
const PAYOUTS = `${BASE}/payouts`;

/**
 * Staff shares of one organization (TEC-917). Shares are written by the API
 * with each sale line and are append-only; a payout posts a staff payment
 * on a cash account and a wrong one is reversed, never edited. A member
 * with only `tenant.earnings.own` gets their own records from every call.
 */
export const staffSharesService = {
  summary(slug: string, period: StaffSharePeriod) {
    return tenantRequest<StaffSharesSummary>(slug, "GET", `${BASE}/summary`, {
      query: period,
    });
  },

  mine(slug: string, period: StaffSharePeriod) {
    // The page explains a missing staff record itself (404).
    return tenantRequest<MyStaffShares>(slug, "GET", `${BASE}/me`, {
      query: period,
      silent: true,
    });
  },

  listRules(slug: string, params: ListStaffShareRulesParams) {
    return tenantRequest<StaffShareRulePage>(slug, "GET", RULES, {
      query: params,
    });
  },

  createRule(slug: string, body: StaffShareRuleInput) {
    return tenantRequest<StaffShareRule>(slug, "POST", RULES, { body });
  },

  updateRule(slug: string, uuid: string, body: StaffShareRuleInput) {
    return tenantRequest<StaffShareRule>(slug, "PATCH", `${RULES}/${uuid}`, {
      body,
    });
  },

  deleteRule(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", `${RULES}/${uuid}`);
  },

  linesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${LINES}/meta`);
  },

  listLines(slug: string, params: ListStaffShareLinesParams) {
    return tenantRequest<StaffShareLinePage>(slug, "GET", LINES, {
      query: params,
    });
  },

  payoutsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${PAYOUTS}/meta`);
  },

  listPayouts(slug: string, params: ListStaffSharePayoutsParams) {
    return tenantRequest<StaffSharePayoutPage>(slug, "GET", PAYOUTS, {
      query: params,
    });
  },

  pay(slug: string, body: StaffSharePayoutInput) {
    return tenantRequest<StaffSharePayout>(slug, "POST", PAYOUTS, { body });
  },

  reversePayout(slug: string, uuid: string, reason: string) {
    return tenantRequest<StaffSharePayout>(
      slug,
      "POST",
      `${PAYOUTS}/${uuid}/reverse`,
      { body: { reason } },
    );
  },
};

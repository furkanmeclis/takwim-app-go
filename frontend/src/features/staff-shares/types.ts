import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type StaffShareRule = Schemas["StaffShareRule"];
export type StaffShareRuleInput = Schemas["StaffShareRuleInput"];
export type StaffShareRulePage = Schemas["StaffShareRulePage"];
export type StaffShareLine = Schemas["StaffShareLine"];
export type StaffShareLinePage = Schemas["StaffShareLinePage"];
export type StaffShareBalance = Schemas["StaffShareBalance"];
export type StaffShareSummaryRow = Schemas["StaffShareSummaryRow"];
export type StaffShareBranchRow = Schemas["StaffShareBranchRow"];
export type StaffShareTotals = Schemas["StaffShareTotals"];
export type StaffSharesSummary = Schemas["StaffSharesSummary"];
export type MyStaffShares = Schemas["MyStaffShares"];
export type StaffSharePayout = Schemas["StaffSharePayout"];
export type StaffSharePayoutInput = Schemas["StaffSharePayoutInput"];
export type StaffSharePayoutPage = Schemas["StaffSharePayoutPage"];

export type StaffShareItemType = StaffShareRule["item_type"];
export type StaffShareBasis = StaffShareRule["basis"];
export type StaffShareStatus = StaffShareLine["status"];
export type StaffShareLineKind = StaffShareLine["kind"];
export type StaffPayoutKind = StaffSharePayout["kind"];

/** A period of the summary endpoints (dates as "2026-06-01"). */
export type StaffSharePeriod = {
  date_from?: string;
  date_to?: string;
};

/** Query of `GET /v1/tenant/staff-shares/rules`. */
export type ListStaffShareRulesParams = {
  limit: number;
  offset: number;
  /** Comma-separated staff uuids. */
  staff?: string;
  item_type?: string;
};

/** Query of `GET /v1/tenant/staff-shares/lines`. */
export type ListStaffShareLinesParams = StaffSharePeriod & {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  staff?: string;
  branch?: string;
  kind?: string;
  item_type?: string;
  status?: StaffShareStatus;
};

/** Query of `GET /v1/tenant/staff-shares/payouts`. */
export type ListStaffSharePayoutsParams = StaffSharePeriod & {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  staff?: string;
  branch?: string;
  kind?: string;
};

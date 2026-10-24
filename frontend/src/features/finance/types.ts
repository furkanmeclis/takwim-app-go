import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type FinanceKind = Schemas["FinanceKind"];
export type FinanceRef = Schemas["FinanceRef"];
export type FinanceCategory = Schemas["FinanceCategory"];
export type FinanceCategoryPage = Schemas["FinanceCategoryPage"];
export type FinanceCategoryCreateRequest =
  Schemas["FinanceCategoryCreateRequest"];
export type FinanceCategoryPatchRequest =
  Schemas["FinanceCategoryPatchRequest"];
export type FinanceDocument = Schemas["FinanceDocument"];
export type FinanceEntry = Schemas["FinanceEntry"];
export type FinanceEntryCreateRequest = Schemas["FinanceEntryCreateRequest"];
export type FinanceEntryPatchRequest = Schemas["FinanceEntryPatchRequest"];
export type FinanceEntryCorrectRequest = Schemas["FinanceEntryCorrectRequest"];
export type FinanceLedgerRow = Schemas["FinanceLedgerRow"];
export type FinanceLedgerPage = Schemas["FinanceLedgerPage"];
export type FinanceFacets = Schemas["FinanceFacets"];
export type FinanceSummary = Schemas["FinanceSummary"];
export type FinanceSummaryTotals = Schemas["FinanceSummaryTotals"];
export type FinanceCategoryTotal = Schemas["FinanceCategoryTotal"];
export type FinanceActivityPage = Schemas["BranchActivityPage"];
export type FinanceActivityEntry = Schemas["BranchActivityEntry"];

export type FinanceSource = FinanceLedgerRow["source"];
export type FinanceEntryType = FinanceEntry["entry_type"];
export type FinanceCategoryStatus = FinanceCategory["status"];

/** A period of the summary (dates as "2026-06-01"). */
export type FinancePeriod = {
  date_from: string;
  date_to: string;
};

/** Query of `GET /v1/tenant/finance/entries` (and its export). */
export type ListFinanceEntriesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated `manual`, `sale`. */
  source?: string;
  /** Comma-separated `income`, `expense`. */
  kind?: string;
  /** Comma-separated category uuids, `sale_service`, `sale_product`. */
  category?: string;
  /** Comma-separated branch uuids; `head_office` = business-wide records. */
  branch?: string;
  /** Comma-separated staff uuids. */
  staff?: string;
  /** Comma-separated cash account uuids. */
  account?: string;
  /** `yyyy-MM-dd`, inclusive. */
  date_from?: string;
  date_to?: string;
};

/** Query of `GET /v1/tenant/finance/categories`. */
export type ListFinanceCategoriesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  kind?: string;
  status?: string;
};

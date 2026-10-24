import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SaleLine = Schemas["SaleLine"];
export type SaleLinePage = Schemas["SaleLinePage"];
export type SaleLineCorrectRequest = Schemas["SaleLineCorrectRequest"];
export type SaleLineCorrection = Schemas["SaleLineCorrection"];
export type LedgerEntry = Schemas["LedgerEntry"];
export type LedgerEntryPage = Schemas["LedgerEntryPage"];
export type SalesSummary = Schemas["SalesSummary"];
export type SalesFigures = Schemas["SalesFigures"];
export type ReservationBalance = Schemas["ReservationBalance"];
export type CustomerBalance = Schemas["CustomerBalance"];
export type LedgerTotals = Schemas["LedgerTotals"];

export type SaleLineKind = SaleLine["kind"];
export type SaleItemType = SaleLine["item_type"];
export type LedgerEntryKind = LedgerEntry["kind"];

/** Period of the ledger page (dates as "2026-06-01", inclusive). */
export type SalesPeriod = {
  date_from: string;
  date_to: string;
};

/** Query of `GET /v1/tenant/sales/lines`. */
export type ListSaleLinesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids. */
  branch?: string;
  /** Comma-separated `sale`, `reversal`. */
  kind?: string;
  /** Comma-separated `service`, `product`. */
  item_type?: string;
  /** Comma-separated staff uuids. */
  staff?: string;
  reservation?: string;
  customer?: string;
  date_from?: string;
  date_to?: string;
};

/** Query of `GET /v1/tenant/sales/entries`. */
export type ListLedgerEntriesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids. */
  branch?: string;
  /** Comma-separated entry kinds. */
  kind?: string;
  reservation?: string;
  customer?: string;
  date_from?: string;
  date_to?: string;
};

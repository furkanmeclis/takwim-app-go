import type { ResourceMeta } from "@/features/io/types";
import type {
  CustomerBalance,
  LedgerEntry,
  LedgerEntryPage,
  ListLedgerEntriesParams,
  ListSaleLinesParams,
  ReservationBalance,
  SaleLine,
  SaleLineCorrectRequest,
  SaleLineCorrection,
  SaleLinePage,
  SalesPeriod,
  SalesSummary,
} from "@/features/sales/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/sales";
const LINES = `${BASE}/lines`;
const ENTRIES = `${BASE}/entries`;

/**
 * Sale lines (revenue) and the customer ledger of one organization
 * (TEC-916). Both are append-only: a wrong line is reversed or corrected
 * (reversal + replacement dated today), never edited.
 */
export const salesService = {
  linesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${LINES}/meta`);
  },

  listLines(slug: string, params: ListSaleLinesParams) {
    return tenantRequest<SaleLinePage>(slug, "GET", LINES, { query: params });
  },

  getLine(slug: string, uuid: string) {
    return tenantRequest<SaleLine>(slug, "GET", `${LINES}/${uuid}`);
  },

  reverseLine(slug: string, uuid: string, reason: string) {
    return tenantRequest<SaleLine>(slug, "POST", `${LINES}/${uuid}/reverse`, {
      body: { reason },
    });
  },

  correctLine(slug: string, uuid: string, body: SaleLineCorrectRequest) {
    return tenantRequest<SaleLineCorrection>(
      slug,
      "POST",
      `${LINES}/${uuid}/correct`,
      { body },
    );
  },

  entriesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${ENTRIES}/meta`);
  },

  listEntries(slug: string, params: ListLedgerEntriesParams) {
    return tenantRequest<LedgerEntryPage>(slug, "GET", ENTRIES, {
      query: params,
    });
  },

  getEntry(slug: string, uuid: string) {
    return tenantRequest<LedgerEntry>(slug, "GET", `${ENTRIES}/${uuid}`);
  },

  summary(slug: string, period: SalesPeriod) {
    return tenantRequest<SalesSummary>(slug, "GET", `${BASE}/summary`, {
      query: period,
    });
  },

  reservationBalance(slug: string, uuid: string) {
    return tenantRequest<ReservationBalance>(
      slug,
      "GET",
      `${BASE}/reservations/${uuid}/balance`,
    );
  },

  customerBalance(slug: string, uuid: string) {
    return tenantRequest<CustomerBalance>(
      slug,
      "GET",
      `${BASE}/customers/${uuid}/balance`,
    );
  },
};

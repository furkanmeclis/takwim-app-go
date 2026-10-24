import type {
  Report,
  ReportFilter,
  ReportSection,
} from "@/features/reports/types";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/reports";

function query(filter: ReportFilter): Record<string, string | undefined> {
  return {
    date_from: filter.date_from,
    date_to: filter.date_to,
    branch: filter.branch || undefined,
    staff: filter.staff || undefined,
  };
}

/**
 * Reports of one organization (TEC-925). Read-only figures over the
 * reservations, sale lines, ledger, cash book, staff shares and stock; the
 * branch picked in the header narrows them like every tenant view.
 */
export const reportsService = {
  summary(slug: string, filter: ReportFilter) {
    return tenantRequest<Report>(slug, "GET", `${BASE}/summary`, {
      query: query(filter),
    });
  },

  /** CSV of the same filter (the API names the file). */
  exportCsv(
    slug: string,
    filter: ReportFilter,
    section: ReportSection,
    locale: string,
  ) {
    return tenantDownloadFile(slug, `${BASE}/export`, {
      ...query(filter),
      section,
      locale,
    });
  },
};

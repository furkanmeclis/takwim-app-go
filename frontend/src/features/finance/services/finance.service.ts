import type {
  FinanceActivityPage,
  FinanceCategory,
  FinanceCategoryCreateRequest,
  FinanceCategoryPage,
  FinanceCategoryPatchRequest,
  FinanceEntry,
  FinanceEntryCorrectRequest,
  FinanceEntryCreateRequest,
  FinanceEntryPatchRequest,
  FinanceFacets,
  FinanceLedgerPage,
  FinancePeriod,
  FinanceSummary,
  ListFinanceCategoriesParams,
  ListFinanceEntriesParams,
} from "@/features/finance/types";
import type { ResourceMeta } from "@/features/io/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/finance";
const CATEGORIES = `${BASE}/categories`;
const ENTRIES = `${BASE}/entries`;

/**
 * Income and expenses of one organization (TEC-915). A record posts its
 * cash movement in the same transaction and is never deleted: a wrong one
 * is reversed or corrected (reversal + replacement). Sale lines show up in
 * the list as income on their own. The document is streamed by the API.
 */
export const financeService = {
  listCategories(slug: string, params: ListFinanceCategoriesParams) {
    return tenantRequest<FinanceCategoryPage>(slug, "GET", CATEGORIES, {
      query: params,
    });
  },

  createCategory(slug: string, body: FinanceCategoryCreateRequest) {
    return tenantRequest<FinanceCategory>(slug, "POST", CATEGORIES, { body });
  },

  updateCategory(
    slug: string,
    uuid: string,
    body: FinanceCategoryPatchRequest,
  ) {
    return tenantRequest<FinanceCategory>(
      slug,
      "PATCH",
      `${CATEGORIES}/${uuid}`,
      { body },
    );
  },

  deleteCategory(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", `${CATEGORIES}/${uuid}`);
  },

  entriesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${ENTRIES}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<FinanceFacets>(slug, "GET", `${ENTRIES}/facets`);
  },

  listEntries(slug: string, params: ListFinanceEntriesParams) {
    return tenantRequest<FinanceLedgerPage>(slug, "GET", ENTRIES, {
      query: params,
    });
  },

  getEntry(slug: string, uuid: string) {
    return tenantRequest<FinanceEntry>(slug, "GET", `${ENTRIES}/${uuid}`);
  },

  createEntry(slug: string, body: FinanceEntryCreateRequest) {
    return tenantRequest<FinanceEntry>(slug, "POST", ENTRIES, { body });
  },

  updateEntry(slug: string, uuid: string, body: FinanceEntryPatchRequest) {
    return tenantRequest<FinanceEntry>(slug, "PATCH", `${ENTRIES}/${uuid}`, {
      body,
    });
  },

  reverseEntry(slug: string, uuid: string, reason: string) {
    return tenantRequest<FinanceEntry>(
      slug,
      "POST",
      `${ENTRIES}/${uuid}/reverse`,
      { body: { reason } },
    );
  },

  correctEntry(slug: string, uuid: string, body: FinanceEntryCorrectRequest) {
    return tenantRequest<FinanceEntry>(
      slug,
      "POST",
      `${ENTRIES}/${uuid}/correct`,
      { body },
    );
  },

  activity(slug: string, uuid: string, params: Record<string, number>) {
    return tenantRequest<FinanceActivityPage>(
      slug,
      "GET",
      `${ENTRIES}/${uuid}/activity`,
      { query: params },
    );
  },

  uploadDocument(slug: string, uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<FinanceEntry>(
      slug,
      "PUT",
      `${ENTRIES}/${uuid}/document`,
      form,
    );
  },

  downloadDocument(slug: string, uuid: string) {
    return tenantDownloadFile(slug, `${ENTRIES}/${uuid}/document`);
  },

  removeDocument(slug: string, uuid: string) {
    return tenantRequest<FinanceEntry>(
      slug,
      "DELETE",
      `${ENTRIES}/${uuid}/document`,
    );
  },

  summary(slug: string, period: FinancePeriod) {
    return tenantRequest<FinanceSummary>(slug, "GET", `${BASE}/summary`, {
      query: period,
    });
  },
};

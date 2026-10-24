import type {
  FinanceCategoryStatus,
  FinanceKind,
  FinanceSource,
} from "@/features/finance/types";

/** Engine / live slugs (io, activity, realtime). */
export const FINANCE_ENTRIES_RESOURCE = "tenant.finance_entries" as const;
export const FINANCE_CATEGORIES_RESOURCE = "tenant.finance_categories" as const;

export const FINANCE_KINDS: FinanceKind[] = ["income", "expense"];
export const FINANCE_SOURCES: FinanceSource[] = ["manual", "sale"];
export const FINANCE_CATEGORY_STATUSES: FinanceCategoryStatus[] = [
  "active",
  "archived",
];

/** Category filter values of the sale rows (automatic income). */
export const SALE_SERVICE = "sale_service";
export const SALE_PRODUCT = "sale_product";

/** Branch filter value of business-wide records. */
export const HEAD_OFFICE = "head_office";

/** API limits (see `FinanceEntryCreateRequest`). */
export const MAX_DESCRIPTION = 500;
export const MAX_CATEGORY_NAME = 120;
export const MAX_DOCUMENT_BYTES = 10 * 1024 * 1024;
export const DOCUMENT_ACCEPT =
  "application/pdf,image/jpeg,image/png,image/webp";

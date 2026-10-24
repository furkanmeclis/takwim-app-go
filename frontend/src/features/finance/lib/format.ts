import {
  HEAD_OFFICE,
  SALE_PRODUCT,
  SALE_SERVICE,
} from "@/features/finance/constants";
import type { FinanceKind, FinanceLedgerRow } from "@/features/finance/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export const kindLabel = (t: Translate, kind: FinanceKind) =>
  t(`finance.kinds.${kind}`);

export const sourceLabel = (t: Translate, source: FinanceLedgerRow["source"]) =>
  t(`finance.sources.${source}`);

/** Facet / filter value of a category: a uuid or one of the sale rows. */
export function categoryValueLabel(
  t: Translate,
  value: string,
  label?: string,
) {
  if (value === SALE_SERVICE) return t("finance.sale_categories.service");
  if (value === SALE_PRODUCT) return t("finance.sale_categories.product");
  return label ?? value;
}

/** Category of a list row; sale rows are "Hizmet satışı" / "Ürün satışı". */
export function rowCategoryLabel(t: Translate, row: FinanceLedgerRow) {
  if (row.source === "sale") {
    return row.item_type === "product"
      ? t("finance.sale_categories.product")
      : t("finance.sale_categories.service");
  }
  return row.category?.name ?? "—";
}

export function branchValueLabel(t: Translate, value: string, label?: string) {
  return value === HEAD_OFFICE ? t("finance.head_office") : (label ?? value);
}

/** Expenses read as money out: "-150,00 ₺". */
export function signedAmount(kind: FinanceKind, amount: string) {
  if (kind !== "expense") return amount;
  return amount.startsWith("-") ? amount.slice(1) : `-${amount}`;
}

import type {
  LedgerEntryKind,
  SaleItemType,
  SaleLineKind,
} from "@/features/sales/types";

/** Live list slugs (org:{uuid}:resource:…). */
export const SALE_LINES_RESOURCE = "tenant.sale_lines" as const;
export const LEDGER_ENTRIES_RESOURCE = "tenant.ledger_entries" as const;

export const SALE_LINE_KINDS: SaleLineKind[] = ["sale", "reversal"];
export const SALE_ITEM_TYPES: SaleItemType[] = ["service", "product"];
export const LEDGER_ENTRY_KINDS: LedgerEntryKind[] = [
  "sale",
  "package",
  "collection",
  "refund",
  "reversal",
];

export const LINE_KIND_LABEL_KEYS: Record<SaleLineKind, string> = {
  sale: "sales.line_kinds.sale",
  reversal: "sales.line_kinds.reversal",
};

export const ITEM_TYPE_LABEL_KEYS: Record<SaleItemType, string> = {
  service: "sales.item_types.service",
  product: "sales.item_types.product",
};

export const ENTRY_KIND_LABEL_KEYS: Record<LedgerEntryKind, string> = {
  sale: "sales.entry_kinds.sale",
  package: "sales.entry_kinds.package",
  collection: "sales.entry_kinds.collection",
  refund: "sales.entry_kinds.refund",
  reversal: "sales.entry_kinds.reversal",
};

export const METHOD_LABEL_KEYS: Record<string, string> = {
  cash: "sales.methods.cash",
  card: "sales.methods.card",
  transfer: "sales.methods.transfer",
  online: "sales.methods.online",
  other: "sales.methods.other",
};

export const DIRECTION_LABEL_KEYS: Record<"debit" | "credit", string> = {
  debit: "sales.directions.debit",
  credit: "sales.directions.credit",
};

/** API limit of a corrected quantity. */
export const MAX_QUANTITY = 100000;

import type {
  CashAccountKind,
  CashAccountStatus,
  CashDirection,
  CashMovementKind,
  CashStatementLineStatus,
  CashStatementLineType,
  CashStatementPostKind,
  CashStatementSource,
} from "@/features/cash/types";

/** Engine / live slugs (io, bulk, activity, realtime). */
export const CASH_ACCOUNTS_RESOURCE = "tenant.cash_accounts" as const;
export const CASH_MOVEMENTS_RESOURCE = "tenant.cash_movements" as const;

/** Branch filter value of business-wide accounts (no branch). */
export const HEAD_OFFICE = "head_office";

export const CASH_ACCOUNT_KINDS: CashAccountKind[] = ["cash", "bank", "online"];

export const CASH_ACCOUNT_STATUSES: CashAccountStatus[] = [
  "active",
  "archived",
];

export const CASH_MOVEMENT_KINDS: CashMovementKind[] = [
  "deposit",
  "withdrawal",
  "transfer_in",
  "transfer_out",
  "closing_difference",
  "reversal",
  "collection",
  "refund",
  "income",
  "expense",
  "staff_payment",
];

/** Movements the panel can reverse; module postings are corrected at their source. */
export const REVERSIBLE_KINDS: CashMovementKind[] = [
  "deposit",
  "withdrawal",
  "transfer_in",
  "transfer_out",
];

/** API limits (see `CashAccountCreateRequest`). */
export const MAX_SORT_ORDER = 100000;
export const MAX_DESCRIPTION = 500;
export const MAX_NOTE = 2000;

// ---------------------------------------------------------------- statements

export const CASH_STATEMENTS_RESOURCE = "tenant.cash_statements" as const;
export const CASH_STATEMENT_LINES_RESOURCE =
  "tenant.cash_statement_lines" as const;

export const CASH_STATEMENT_SOURCES: CashStatementSource[] = [
  "bank",
  "provider",
];

export const CASH_STATEMENT_LINE_STATUSES: CashStatementLineStatus[] = [
  "pending",
  "matched",
  "difference",
  "posted",
  "ignored",
];

export const CASH_STATEMENT_LINE_TYPES: CashStatementLineType[] = [
  "payment",
  "refund",
  "fee",
  "payout",
  "other",
];

/** Movement kinds a statement line may be posted as, by direction. */
export const CASH_POST_KINDS: Record<CashDirection, CashStatementPostKind[]> = {
  in: ["income", "collection"],
  out: ["expense", "refund", "staff_payment"],
};

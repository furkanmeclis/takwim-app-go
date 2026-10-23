import type {
  CashAccountKind,
  CashAccountStatus,
  CashMovementKind,
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

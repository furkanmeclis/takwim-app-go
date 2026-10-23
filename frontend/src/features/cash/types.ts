import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type CashAccount = Schemas["CashAccount"];
export type CashAccountPage = Schemas["CashAccountPage"];
export type CashAccountFacets = Schemas["CashAccountFacets"];
export type CashAccountOption = Schemas["CashAccountOption"];
export type CashAccountOptionList = Schemas["CashAccountOptionList"];
export type CashAccountCreateRequest = Schemas["CashAccountCreateRequest"];
export type CashAccountPatchRequest = Schemas["CashAccountPatchRequest"];
export type CashAccountKind = Schemas["CashAccountKind"];
export type CashMovement = Schemas["CashMovement"];
export type CashMovementKind = Schemas["CashMovementKind"];
export type CashMovementPage = Schemas["CashMovementPage"];
export type CashMovementList = Schemas["CashMovementList"];
export type CashMovementFacets = Schemas["CashMovementFacets"];
export type CashMovementCreateRequest = Schemas["CashMovementCreateRequest"];
export type CashTransferRequest = Schemas["CashTransferRequest"];
export type CashTransfer = Schemas["CashTransfer"];
export type CashClosing = Schemas["CashClosing"];
export type CashClosingPage = Schemas["CashClosingPage"];
export type CashClosingPreview = Schemas["CashClosingPreview"];
export type CashClosingCreateRequest = Schemas["CashClosingCreateRequest"];
export type CashDailySummary = Schemas["CashDailySummary"];
export type CashDailyAccount = Schemas["CashDailyAccount"];
export type CashDailyTotals = Schemas["CashDailyTotals"];
export type CashActivityPage = Schemas["BranchActivityPage"];
export type CashActivityEntry = Schemas["BranchActivityEntry"];

export type CashDirection = CashMovement["direction"];
export type CashAccountStatus = CashAccount["status"];

/** List query of `GET /v1/tenant/cash/accounts`. */
export type ListCashAccountsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids; `head_office` = business-wide accounts. */
  branch?: string;
  /** Comma-separated kinds. */
  kind?: string;
  /** Comma-separated statuses. */
  status?: string;
};

/** List query of `GET /v1/tenant/cash/movements` (and an account's book). */
export type ListCashMovementsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated account uuids. */
  account?: string;
  /** Comma-separated movement kinds. */
  kind?: string;
  direction?: CashDirection;
  /** `yyyy-MM-dd`, inclusive. */
  date_from?: string;
  date_to?: string;
};

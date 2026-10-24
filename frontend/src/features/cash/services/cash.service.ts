import type {
  CashAccount,
  CashAccountCreateRequest,
  CashAccountFacets,
  CashAccountOptionList,
  CashAccountPage,
  CashAccountPatchRequest,
  CashActivityPage,
  CashClosing,
  CashClosingCreateRequest,
  CashClosingPage,
  CashClosingPreview,
  CashDailySummary,
  CashMovement,
  CashMovementCreateRequest,
  CashMovementFacets,
  CashMovementList,
  CashMovementPage,
  CashAutoMatchResult,
  CashMatchCandidateList,
  CashStatement,
  CashStatementCreateRequest,
  CashStatementLine,
  CashStatementLineCreateRequest,
  CashStatementLineFacets,
  CashStatementLineMatchRequest,
  CashStatementLinePage,
  CashStatementLinePostRequest,
  CashStatementPage,
  CashStatementSummary,
  CashStatementUpdateRequest,
  CashTransfer,
  CashTransferRequest,
  ListCashAccountsParams,
  ListCashMovementsParams,
  ListCashStatementLinesParams,
  ListCashStatementsParams,
} from "@/features/cash/types";
import type { ResourceMeta } from "@/features/io/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/cash";
const ACCOUNTS = `${BASE}/accounts`;
const MOVEMENTS = `${BASE}/movements`;
const STATEMENTS = `${BASE}/statements`;
const LINES = `${BASE}/statement-lines`;

type Paging = { limit: number; offset: number };

/**
 * Cash book of one organization (TEC-914). Every call names the
 * organization through `tenantRequest`; the API keeps reads and writes in
 * it and in the member's branches. Movements are append-only: a mistake is
 * corrected with a reversal, never edited.
 */
export const cashService = {
  listAccounts(slug: string, params: ListCashAccountsParams) {
    return tenantRequest<CashAccountPage>(slug, "GET", ACCOUNTS, {
      query: params,
    });
  },

  accountsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${ACCOUNTS}/meta`);
  },

  accountFacets(slug: string) {
    return tenantRequest<CashAccountFacets>(slug, "GET", `${ACCOUNTS}/facets`);
  },

  accountOptions(slug: string) {
    return tenantRequest<CashAccountOptionList>(
      slug,
      "GET",
      `${ACCOUNTS}/options`,
    );
  },

  getAccount(slug: string, uuid: string) {
    return tenantRequest<CashAccount>(slug, "GET", `${ACCOUNTS}/${uuid}`);
  },

  createAccount(slug: string, body: CashAccountCreateRequest) {
    return tenantRequest<CashAccount>(slug, "POST", ACCOUNTS, { body });
  },

  updateAccount(slug: string, uuid: string, body: CashAccountPatchRequest) {
    return tenantRequest<CashAccount>(slug, "PATCH", `${ACCOUNTS}/${uuid}`, {
      body,
    });
  },

  accountActivity(slug: string, uuid: string, params: Paging) {
    return tenantRequest<CashActivityPage>(
      slug,
      "GET",
      `${ACCOUNTS}/${uuid}/activity`,
      { query: params },
    );
  },

  accountMovements(
    slug: string,
    uuid: string,
    params: ListCashMovementsParams,
  ) {
    return tenantRequest<CashMovementPage>(
      slug,
      "GET",
      `${ACCOUNTS}/${uuid}/movements`,
      { query: params },
    );
  },

  createMovement(slug: string, uuid: string, body: CashMovementCreateRequest) {
    return tenantRequest<CashMovement>(
      slug,
      "POST",
      `${ACCOUNTS}/${uuid}/movements`,
      { body },
    );
  },

  listClosings(slug: string, uuid: string, params: Paging) {
    return tenantRequest<CashClosingPage>(
      slug,
      "GET",
      `${ACCOUNTS}/${uuid}/closings`,
      { query: params },
    );
  },

  previewClosing(slug: string, uuid: string, date?: string) {
    return tenantRequest<CashClosingPreview>(
      slug,
      "GET",
      `${ACCOUNTS}/${uuid}/closings/preview`,
      { query: { date } },
    );
  },

  closeDay(slug: string, uuid: string, body: CashClosingCreateRequest) {
    return tenantRequest<CashClosing>(
      slug,
      "POST",
      `${ACCOUNTS}/${uuid}/closings`,
      { body },
    );
  },

  reopenDay(slug: string, uuid: string, closingUuid: string, reason: string) {
    return tenantRequest<CashClosing>(
      slug,
      "POST",
      `${ACCOUNTS}/${uuid}/closings/${closingUuid}/reopen`,
      { body: { reason } },
    );
  },

  listMovements(slug: string, params: ListCashMovementsParams) {
    return tenantRequest<CashMovementPage>(slug, "GET", MOVEMENTS, {
      query: params,
    });
  },

  movementsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${MOVEMENTS}/meta`);
  },

  movementFacets(slug: string) {
    return tenantRequest<CashMovementFacets>(
      slug,
      "GET",
      `${MOVEMENTS}/facets`,
    );
  },

  reverseMovement(slug: string, uuid: string, reason: string) {
    return tenantRequest<CashMovementList>(
      slug,
      "POST",
      `${MOVEMENTS}/${uuid}/reverse`,
      { body: { reason } },
    );
  },

  transfer(slug: string, body: CashTransferRequest) {
    return tenantRequest<CashTransfer>(slug, "POST", `${BASE}/transfers`, {
      body,
    });
  },

  dailySummary(slug: string, params: { date?: string; branch?: string }) {
    return tenantRequest<CashDailySummary>(
      slug,
      "GET",
      `${BASE}/daily-summary`,
      { query: params },
    );
  },

  // ---------------------------------------------------------- statements

  listStatements(slug: string, params: ListCashStatementsParams) {
    return tenantRequest<CashStatementPage>(slug, "GET", STATEMENTS, {
      query: params,
    });
  },

  statementsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${STATEMENTS}/meta`);
  },

  getStatement(slug: string, uuid: string) {
    return tenantRequest<CashStatement>(slug, "GET", `${STATEMENTS}/${uuid}`);
  },

  createStatement(slug: string, body: CashStatementCreateRequest) {
    return tenantRequest<CashStatement>(slug, "POST", STATEMENTS, { body });
  },

  updateStatement(
    slug: string,
    uuid: string,
    body: CashStatementUpdateRequest,
  ) {
    return tenantRequest<CashStatement>(
      slug,
      "PATCH",
      `${STATEMENTS}/${uuid}`,
      { body },
    );
  },

  deleteStatement(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", `${STATEMENTS}/${uuid}`);
  },

  statementSummary(slug: string, uuid: string) {
    return tenantRequest<CashStatementSummary>(
      slug,
      "GET",
      `${STATEMENTS}/${uuid}/summary`,
    );
  },

  statementLines(
    slug: string,
    uuid: string,
    params: ListCashStatementLinesParams,
  ) {
    return tenantRequest<CashStatementLinePage>(
      slug,
      "GET",
      `${STATEMENTS}/${uuid}/lines`,
      { query: params },
    );
  },

  statementLineFacets(slug: string, uuid: string) {
    return tenantRequest<CashStatementLineFacets>(
      slug,
      "GET",
      `${STATEMENTS}/${uuid}/lines/facets`,
    );
  },

  addStatementLine(
    slug: string,
    uuid: string,
    body: CashStatementLineCreateRequest,
  ) {
    return tenantRequest<CashStatementLine>(
      slug,
      "POST",
      `${STATEMENTS}/${uuid}/lines`,
      { body },
    );
  },

  autoMatch(slug: string, uuid: string) {
    return tenantRequest<CashAutoMatchResult>(
      slug,
      "POST",
      `${STATEMENTS}/${uuid}/auto-match`,
      { body: {} },
    );
  },

  linesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${LINES}/meta`);
  },

  lineCandidates(slug: string, lineUuid: string, q?: string) {
    return tenantRequest<CashMatchCandidateList>(
      slug,
      "GET",
      `${LINES}/${lineUuid}/candidates`,
      { query: { q: q || undefined } },
    );
  },

  matchLine(
    slug: string,
    lineUuid: string,
    body: CashStatementLineMatchRequest,
  ) {
    return tenantRequest<CashStatementLine>(
      slug,
      "POST",
      `${LINES}/${lineUuid}/match`,
      { body },
    );
  },

  resetLine(slug: string, lineUuid: string, reason?: string) {
    return tenantRequest<CashStatementLine>(
      slug,
      "POST",
      `${LINES}/${lineUuid}/reset`,
      { body: { reason: reason ?? "" } },
    );
  },

  postLine(slug: string, lineUuid: string, body: CashStatementLinePostRequest) {
    return tenantRequest<CashStatementLine>(
      slug,
      "POST",
      `${LINES}/${lineUuid}/post`,
      { body },
    );
  },

  ignoreLine(slug: string, lineUuid: string, reason: string) {
    return tenantRequest<CashStatementLine>(
      slug,
      "POST",
      `${LINES}/${lineUuid}/ignore`,
      { body: { reason } },
    );
  },

  reverseLine(slug: string, lineUuid: string, reason: string) {
    return tenantRequest<CashStatementLine>(
      slug,
      "POST",
      `${LINES}/${lineUuid}/reverse`,
      { body: { reason } },
    );
  },
};

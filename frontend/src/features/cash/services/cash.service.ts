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
  CashTransfer,
  CashTransferRequest,
  ListCashAccountsParams,
  ListCashMovementsParams,
} from "@/features/cash/types";
import type { ResourceMeta } from "@/features/io/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/cash";
const ACCOUNTS = `${BASE}/accounts`;
const MOVEMENTS = `${BASE}/movements`;

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
};

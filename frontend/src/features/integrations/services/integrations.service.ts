import type {
  ConnectIntegrationRequest,
  IntegrationAccount,
  IntegrationAccountPage,
  IntegrationJobDetail,
  IntegrationJobPage,
  IntegrationJobRetry,
  IntegrationsMeta,
  ListIntegrationJobsParams,
  ListIntegrationsParams,
  UpdateIntegrationRequest,
} from "@/features/integrations/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const INTEGRATIONS = "/v1/tenant/integrations";
const INTEGRATION_JOBS = "/v1/tenant/integration-jobs";

/**
 * Provider accounts and integration jobs (`tenantRequest` only; same-origin
 * BFF). Calls whose errors the dialogs render themselves are `silent`.
 */
export const integrationsService = {
  meta(slug: string) {
    return tenantRequest<IntegrationsMeta>(slug, "GET", `${INTEGRATIONS}/meta`);
  },
  list(slug: string, params: ListIntegrationsParams) {
    return tenantRequest<IntegrationAccountPage>(slug, "GET", INTEGRATIONS, {
      query: params,
    });
  },
  connect(slug: string, body: ConnectIntegrationRequest) {
    return tenantRequest<IntegrationAccount>(slug, "POST", INTEGRATIONS, {
      body,
      silent: true,
    });
  },
  update(slug: string, uuid: string, body: UpdateIntegrationRequest) {
    return tenantRequest<IntegrationAccount>(
      slug,
      "PATCH",
      `${INTEGRATIONS}/${uuid}`,
      { body, silent: true },
    );
  },
  test(slug: string, uuid: string) {
    return tenantRequest<IntegrationAccount>(
      slug,
      "POST",
      `${INTEGRATIONS}/${uuid}/test`,
      { body: {}, silent: true },
    );
  },
  setStatus(slug: string, uuid: string, action: "enable" | "disable") {
    return tenantRequest<IntegrationAccount>(
      slug,
      "POST",
      `${INTEGRATIONS}/${uuid}/${action}`,
      { body: {} },
    );
  },
  /** An account's jobs, or every job of the organization (account null). */
  jobs(
    slug: string,
    account: string | null,
    params: ListIntegrationJobsParams,
  ) {
    const path = account ? `${INTEGRATIONS}/${account}/jobs` : INTEGRATION_JOBS;
    return tenantRequest<IntegrationJobPage>(slug, "GET", path, {
      query: params,
    });
  },
  job(slug: string, uuid: string) {
    return tenantRequest<IntegrationJobDetail>(
      slug,
      "GET",
      `${INTEGRATION_JOBS}/${uuid}`,
    );
  },
  /** CheckStatus first; resubmits only when the provider confirms "not done". */
  retry(slug: string, uuid: string) {
    return tenantRequest<IntegrationJobRetry>(
      slug,
      "POST",
      `${INTEGRATION_JOBS}/${uuid}/retry`,
      { body: {}, silent: true },
    );
  },
};

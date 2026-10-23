import type {
  BulkExecuteSyncResult,
  BulkJob,
  BulkResource,
  BulkTarget,
  ListJobsResult,
} from "@/features/bulk-engine/types";
import { BULK_PATHS } from "@/features/bulk-engine/types";
import {
  PLATFORM_SCOPE,
  scopedJobPath,
  scopedRequest,
  type ApiScope,
} from "@/lib/api/scoped-request";

/**
 * Bulk API. `scope` picks the shell: platform jobs live under
 * `/v1/platform/bulk`, organization jobs under `/v1/tenant/bulk`.
 */
export const bulkService = {
  async execute(
    resource: BulkResource,
    body: { action: string; target: BulkTarget; locale: string },
    scope: ApiScope = PLATFORM_SCOPE,
  ) {
    return scopedRequest<BulkJob | BulkExecuteSyncResult>(
      scope,
      "POST",
      BULK_PATHS[resource],
      { body },
    );
  },

  async get(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<BulkJob>(
      scope,
      "GET",
      `${scopedJobPath(scope, "bulk")}/${uuid}`,
    );
  },

  async rollback(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<BulkJob>(
      scope,
      "POST",
      `${scopedJobPath(scope, "bulk")}/${uuid}/rollback`,
    );
  },

  async list(params: { limit?: number; offset?: number }) {
    return scopedRequest<ListJobsResult>(
      PLATFORM_SCOPE,
      "GET",
      "/v1/platform/bulk",
      { query: params },
    );
  },
};

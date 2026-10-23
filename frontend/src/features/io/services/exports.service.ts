import type { ExportFormat, ExportJob } from "@/features/io/types";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import {
  PLATFORM_SCOPE,
  scopedDownloadFile,
  scopedJobPath,
  scopedRequest,
  type ApiScope,
} from "@/lib/api/scoped-request";
import { exportDownloadFilename } from "@/features/io/lib/display";

export type ListJobsResult = {
  items: ExportJob[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * Export API. `scope` picks the shell: platform jobs under
 * `/v1/platform/exports`, organization jobs under `/v1/tenant/exports`.
 */
export const exportsService = {
  async request(
    path: string,
    body: {
      format: ExportFormat;
      query?: Record<string, string>;
      locale?: string;
    },
    scope: ApiScope = PLATFORM_SCOPE,
  ) {
    return scopedRequest<ExportJob>(scope, "POST", path, { body });
  },

  async list(params: { limit?: number; offset?: number }) {
    return scopedRequest<ListJobsResult>(
      PLATFORM_SCOPE,
      "GET",
      "/v1/platform/exports",
      { query: params },
    );
  },

  async get(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<ExportJob>(
      scope,
      "GET",
      `${scopedJobPath(scope, "exports")}/${uuid}`,
    );
  },

  async fetchFile(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedDownloadFile(
      scope,
      `${scopedJobPath(scope, "exports")}/${uuid}/download`,
    );
  },

  async download(job: ExportJob, scope: ApiScope = PLATFORM_SCOPE) {
    const { blob, filename } = await this.fetchFile(job.uuid, scope);
    const name =
      filename ??
      exportDownloadFilename(
        job.resource,
        job.format,
        new Date(job.created_at),
      );
    triggerBrowserDownload(blob, name);
  },
};

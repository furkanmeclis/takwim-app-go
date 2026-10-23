import type { ImportFormat, ImportJob } from "@/features/io/types";
import {
  PLATFORM_SCOPE,
  scopedDownloadFile,
  scopedFormRequest,
  scopedJobPath,
  scopedRequest,
  type ApiScope,
} from "@/lib/api/scoped-request";

export type ListJobsResult = {
  items: ImportJob[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * Import API. `scope` picks the shell: platform jobs under
 * `/v1/platform/imports`, organization jobs under `/v1/tenant/imports`.
 */
export const importsService = {
  async upload(
    path: string,
    file: File,
    format: ImportFormat,
    locale: string,
    scope: ApiScope = PLATFORM_SCOPE,
  ) {
    const form = new FormData();
    form.append("file", file);
    form.append("format", format);
    form.append("locale", locale);
    return scopedFormRequest<ImportJob>(scope, "POST", path, form);
  },

  async downloadSample(
    path: string,
    format: ImportFormat,
    locale: string,
    scope: ApiScope = PLATFORM_SCOPE,
  ) {
    const { blob } = await scopedDownloadFile(scope, path, { format, locale });
    return blob;
  },

  async updateMapping(
    uuid: string,
    body: {
      mapping: Record<string, string>;
      defaults?: Record<string, string>;
    },
    scope: ApiScope = PLATFORM_SCOPE,
  ) {
    return scopedRequest<ImportJob>(
      scope,
      "PATCH",
      `${scopedJobPath(scope, "imports")}/${uuid}/mapping`,
      { body },
    );
  },

  async preview(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<ImportJob>(
      scope,
      "POST",
      `${scopedJobPath(scope, "imports")}/${uuid}/preview`,
    );
  },

  async confirm(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<ImportJob>(
      scope,
      "POST",
      `${scopedJobPath(scope, "imports")}/${uuid}/confirm`,
    );
  },

  async rollback(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<ImportJob>(
      scope,
      "POST",
      `${scopedJobPath(scope, "imports")}/${uuid}/rollback`,
    );
  },

  async list(params: { limit?: number; offset?: number }) {
    return scopedRequest<ListJobsResult>(
      PLATFORM_SCOPE,
      "GET",
      "/v1/platform/imports",
      { query: params },
    );
  },

  async get(uuid: string, scope: ApiScope = PLATFORM_SCOPE) {
    return scopedRequest<ImportJob>(
      scope,
      "GET",
      `${scopedJobPath(scope, "imports")}/${uuid}`,
    );
  },
};

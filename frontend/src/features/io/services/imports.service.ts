import type { ImportFormat, ImportJob } from "@/features/io/types";
import {
  platformDownloadRequest,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type ListJobsResult = {
  items: ImportJob[];
  total: number;
  limit: number;
  offset: number;
};

export const importsService = {
  async upload(path: string, file: File, format: ImportFormat, locale: string) {
    const form = new FormData();
    form.append("file", file);
    form.append("format", format);
    form.append("locale", locale);
    return platformFormRequest<ImportJob>("POST", path, form);
  },

  async downloadSample(path: string, format: ImportFormat, locale: string) {
    return platformDownloadRequest(path, { format, locale });
  },

  async updateMapping(
    uuid: string,
    body: {
      mapping: Record<string, string>;
      defaults?: Record<string, string>;
    },
  ) {
    return platformRequest<ImportJob>(
      "PATCH",
      `/v1/platform/imports/${uuid}/mapping`,
      { body },
    );
  },

  async preview(uuid: string) {
    return platformRequest<ImportJob>(
      "POST",
      `/v1/platform/imports/${uuid}/preview`,
    );
  },

  async confirm(uuid: string) {
    return platformRequest<ImportJob>(
      "POST",
      `/v1/platform/imports/${uuid}/confirm`,
    );
  },

  async rollback(uuid: string) {
    return platformRequest<ImportJob>(
      "POST",
      `/v1/platform/imports/${uuid}/rollback`,
    );
  },

  async list(params: { limit?: number; offset?: number }) {
    return platformRequest<ListJobsResult>("GET", "/v1/platform/imports", {
      query: params,
    });
  },

  async get(uuid: string) {
    return platformRequest<ImportJob>("GET", `/v1/platform/imports/${uuid}`);
  },
};

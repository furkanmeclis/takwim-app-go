import type { ExportFormat, ExportJob } from "@/features/io/types";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";
import { exportDownloadFilename } from "@/features/io/lib/display";

export type ListJobsResult = {
  items: ExportJob[];
  total: number;
  limit: number;
  offset: number;
};

export const exportsService = {
  async request(
    path: string,
    body: {
      format: ExportFormat;
      query?: Record<string, string>;
      locale?: string;
    },
  ) {
    return platformRequest<ExportJob>("POST", path, { body });
  },

  async list(params: { limit?: number; offset?: number }) {
    return platformRequest<ListJobsResult>("GET", "/v1/platform/exports", {
      query: params,
    });
  },

  async get(uuid: string) {
    return platformRequest<ExportJob>("GET", `/v1/platform/exports/${uuid}`);
  },

  async fetchFile(uuid: string) {
    return platformDownloadFile(`/v1/platform/exports/${uuid}/download`);
  },

  async download(job: ExportJob) {
    const { blob, filename } = await this.fetchFile(job.uuid);
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

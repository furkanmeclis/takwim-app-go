import { apiClient, unwrap } from "@/lib/api";
import type { SystemInfo } from "@/features/settings/types";

export const systemService = {
  async getInfo() {
    return unwrap<SystemInfo>(await apiClient.GET("/v1/platform/system/info"));
  },

  async ping() {
    return unwrap<SystemInfo>(await apiClient.POST("/v1/platform/system/ping"));
  },
};

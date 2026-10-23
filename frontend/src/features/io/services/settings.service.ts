import type { AppSettings } from "@/features/io/types";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type PatchSettingsRequest = Partial<Omit<AppSettings, "logo_url">>;

export const settingsService = {
  async get() {
    return platformRequest<AppSettings>("GET", "/v1/platform/settings");
  },

  async patch(body: PatchSettingsRequest) {
    return platformRequest<AppSettings>("PATCH", "/v1/platform/settings", {
      body,
    });
  },

  async uploadLogo(file: File) {
    const form = new FormData();
    form.append("logo", file);
    return platformFormRequest<AppSettings>(
      "PUT",
      "/v1/platform/settings/logo",
      form,
    );
  },

  async deleteLogo() {
    return platformRequest<AppSettings>("DELETE", "/v1/platform/settings/logo");
  },
};

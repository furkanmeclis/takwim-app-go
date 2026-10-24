import type {
  ConfirmationSaveInput,
  ConfirmationSettings,
} from "@/features/confirmations/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/confirmations";

/** Customer confirmation settings of one organization (TEC-921). */
export const confirmationsService = {
  settings(slug: string) {
    return tenantRequest<ConfirmationSettings>(slug, "GET", `${BASE}/settings`);
  },

  save(slug: string, body: ConfirmationSaveInput) {
    return tenantRequest<ConfirmationSettings>(
      slug,
      "PUT",
      `${BASE}/settings`,
      { body },
    );
  },

  /** No branch: the business-wide rule goes back to the template. */
  reset(slug: string, branchUuid: string | null) {
    return tenantRequest<ConfirmationSettings>(
      slug,
      "POST",
      `${BASE}/settings/reset`,
      { body: { branch_uuid: branchUuid } },
    );
  },
};

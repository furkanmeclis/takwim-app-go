import type {
  ConfirmationAttempt,
  ConfirmationConfirmInput,
  ConfirmationFacetParams,
  ConfirmationFacets,
  ConfirmationSaveInput,
  ConfirmationSettings,
  ConfirmationSummary,
  ConfirmationTaskPage,
  ConfirmationTexts,
  ConfirmationTextsInput,
  ListConfirmationsParams,
} from "@/features/confirmations/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/confirmations";
const RESERVATIONS = "/v1/tenant/reservations";

/** Customer confirmations of one organization (TEC-921, TEC-924). */
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

  /** The request's SMS and e-mail texts with their defaults (TEC-979). */
  texts(slug: string) {
    return tenantRequest<ConfirmationTexts>(slug, "GET", `${BASE}/texts`);
  },

  saveTexts(slug: string, body: ConfirmationTextsInput) {
    return tenantRequest<ConfirmationTexts>(slug, "PUT", `${BASE}/texts`, {
      body,
    });
  },

  resetTexts(slug: string) {
    return tenantRequest<ConfirmationTexts>(
      slug,
      "POST",
      `${BASE}/texts/reset`,
    );
  },

  /** The confirmation list: bookings with a confirmation task. */
  list(slug: string, params: ListConfirmationsParams) {
    return tenantRequest<ConfirmationTaskPage>(slug, "GET", BASE, {
      query: params,
    });
  },

  facets(slug: string, params: ConfirmationFacetParams) {
    return tenantRequest<ConfirmationFacets>(slug, "GET", `${BASE}/facets`, {
      query: params,
    });
  },

  /** Upcoming visits whose confirmation needs a person (nav, alerts). */
  summary(slug: string) {
    return tenantRequest<ConfirmationSummary>(slug, "GET", `${BASE}/summary`, {
      silent: true,
    });
  },

  attempts(slug: string, reservationUuid: string) {
    return tenantRequest<{ items: ConfirmationAttempt[] }>(
      slug,
      "GET",
      `${RESERVATIONS}/${encodeURIComponent(reservationUuid)}/confirmation/attempts`,
    );
  },

  /** The customer confirmed on the phone. */
  confirm(
    slug: string,
    reservationUuid: string,
    body: ConfirmationConfirmInput,
  ) {
    return tenantRequest<{ uuid: string; confirmation_status: "confirmed" }>(
      slug,
      "POST",
      `${RESERVATIONS}/${encodeURIComponent(reservationUuid)}/confirmation/confirm`,
      { body, silent: true },
    );
  },
};

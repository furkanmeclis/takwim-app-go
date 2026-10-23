import { apiConfig } from "@/config/api";
import { ApiError, parseApiError } from "@/lib/api/errors";
import type {
  PublicAvailability,
  PublicAvailabilityRequest,
  PublicBookingRequest,
  PublicBranch,
  PublicBusiness,
  PublicCheckout,
  PublicCheckoutRequest,
  PublicReservation,
  PublicService,
  PublicStaff,
} from "@/features/business-page/types";

/** An API error with the `Retry-After` seconds of a `429`. */
export class PublicBookingError extends ApiError {
  readonly retryAfter?: number;

  constructor(base: ApiError, retryAfter?: number) {
    super({
      status: base.status,
      code: base.code,
      message: base.message,
      details: base.details,
      requestId: base.requestId,
      body: base.body,
    });
    this.name = "PublicBookingError";
    this.retryAfter = retryAfter;
  }
}

type RequestOptions = {
  query?: Record<string, string | undefined>;
  body?: unknown;
  headers?: Record<string, string>;
};

const base = (slug: string) =>
  `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/public/businesses/${encodeURIComponent(slug)}`;

/**
 * Same-origin BFF call of the public booking API. Cookies are left out
 * (`credentials: "omit"`): the page is anonymous even when a panel user
 * is signed in on the same browser, and no session refresh runs.
 */
async function request<T>(
  method: string,
  slug: string,
  path: string,
  options?: RequestOptions,
): Promise<T> {
  let url = `${base(slug)}${path}`;
  if (options?.query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(options.query)) {
      if (value) params.set(key, value);
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  const response = await fetch(url, {
    method,
    credentials: "omit",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(options?.body !== undefined
        ? { "Content-Type": "application/json" }
        : {}),
      ...options?.headers,
    },
    body: options?.body !== undefined ? JSON.stringify(options.body) : null,
  });
  const payload = (await response.json().catch(() => undefined)) as
    { data?: T } | undefined;
  if (!response.ok) {
    const retry = Number(response.headers.get("Retry-After"));
    throw new PublicBookingError(
      parseApiError(response.status, payload),
      Number.isFinite(retry) && retry > 0 ? retry : undefined,
    );
  }
  return payload?.data as T;
}

/** The customer-facing booking API of one business (`{slug}`). */
export const publicBookingService = {
  business(slug: string) {
    return request<PublicBusiness>("GET", slug, "");
  },
  branches(slug: string) {
    return request<PublicBranch[]>("GET", slug, "/branches");
  },
  services(slug: string, branchUuid?: string) {
    return request<PublicService[]>("GET", slug, "/services", {
      query: { branch_uuid: branchUuid },
    });
  },
  staff(slug: string, branchUuid?: string) {
    return request<PublicStaff[]>("GET", slug, "/staff", {
      query: { branch_uuid: branchUuid },
    });
  },
  availability(slug: string, body: PublicAvailabilityRequest) {
    return request<PublicAvailability>("POST", slug, "/availability", {
      body,
    });
  },
  /** Holds the picked slot; the key keeps a retried call from holding twice. */
  createCheckout(
    slug: string,
    body: PublicCheckoutRequest,
    idempotencyKey: string,
  ) {
    return request<PublicCheckout>("POST", slug, "/checkouts", {
      body,
      headers: { "Idempotency-Key": idempotencyKey },
    });
  },
  releaseCheckout(slug: string, token: string) {
    return request<PublicCheckout>(
      "DELETE",
      slug,
      `/checkouts/${encodeURIComponent(token)}`,
    );
  },
  book(slug: string, body: PublicBookingRequest, idempotencyKey: string) {
    return request<PublicReservation>("POST", slug, "/reservations", {
      body,
      headers: { "Idempotency-Key": idempotencyKey },
    });
  },
  reservation(slug: string, uuid: string, accessToken: string) {
    return request<PublicReservation>(
      "GET",
      slug,
      `/reservations/${encodeURIComponent(uuid)}`,
      { headers: { "X-Reservation-Token": accessToken } },
    );
  },
};

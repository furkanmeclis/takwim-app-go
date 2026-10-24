import { apiConfig } from "@/config/api";
import type {
  CustomerAvailability,
  CustomerDeviceList,
  CustomerListParams,
  CustomerPaymentSummary,
  CustomerProfile,
  CustomerReservation,
  CustomerReservationPage,
  CustomerSession,
  CustomerSignInStart,
} from "@/features/customer-panel/types";
import { parseApiError } from "@/lib/api/errors";
import { uuid as newKey } from "@/lib/utils/format";

/**
 * Customer panel calls go to the same-origin BFF (`/api/v1/customer/*`),
 * which adds the customer session cookie as Bearer. They never use the
 * staff client: a 401 here means the customer session ended, not that a
 * staff token needs a refresh.
 */
const PREFIX = "/v1/customer";

type Options = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
  /** Sends an Idempotency-Key, so a retried change runs once. */
  idempotent?: boolean;
};

let onUnauthorized: (() => void) | null = null;

/** The panel shell sends the person to sign in when the session ends. */
export function setCustomerUnauthorizedHandler(handler: (() => void) | null) {
  onUnauthorized = handler;
}

async function customerRequest<T>(
  method: string,
  path: string,
  options: Options = {},
): Promise<T> {
  let url = `${apiConfig.baseUrl.replace(/\/$/, "")}${PREFIX}${path}`;
  if (options.query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(options.query)) {
      if (value !== undefined && value !== "") params.set(key, String(value));
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (options.idempotent) headers["Idempotency-Key"] = newKey();

  const response = await fetch(url, {
    method,
    credentials: "include",
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });
  if (response.status === 204) return undefined as T;
  const payload = await response.json().catch(() => undefined);
  if (!response.ok) {
    if (response.status === 401 && !path.startsWith("/auth/")) {
      onUnauthorized?.();
    }
    throw parseApiError(response.status, payload);
  }
  return (payload as { data: T }).data;
}

export const customerPanelService = {
  start: (phone: string) =>
    customerRequest<CustomerSignInStart>("POST", "/auth/start", {
      body: { phone },
    }),
  verify: (phone: string, code: string) =>
    customerRequest<CustomerSession>("POST", "/auth/verify", {
      body: { phone, code },
    }),
  /** Exchanges a panel link token (from the URL fragment) for a session. */
  redeemLink: (token: string) =>
    customerRequest<CustomerSession>("POST", "/auth/link", {
      body: { token },
    }),
  logout: (everywhere: boolean) =>
    customerRequest<void>("POST", "/auth/logout", { body: { everywhere } }),
  me: () => customerRequest<CustomerProfile>("GET", "/me"),
  reservations: (params: CustomerListParams) =>
    customerRequest<CustomerReservationPage>("GET", "/reservations", {
      query: params,
    }),
  reservation: (uuid: string) =>
    customerRequest<CustomerReservation>("GET", `/reservations/${uuid}`),
  cancel: (uuid: string, expectedVersion: number) =>
    customerRequest<CustomerReservation>(
      "POST",
      `/reservations/${uuid}/cancel`,
      { body: { expected_version: expectedVersion }, idempotent: true },
    ),
  /** "Geleceğim": answers the business's confirmation request (TEC-921). */
  confirm: (uuid: string) =>
    customerRequest<CustomerReservation>(
      "POST",
      `/reservations/${uuid}/confirm`,
      { body: {}, idempotent: true },
    ),
  requestChange: (uuid: string, note: string) =>
    customerRequest<CustomerReservation>(
      "POST",
      `/reservations/${uuid}/change-request`,
      { body: { note }, idempotent: true },
    ),
  availability: (uuid: string, from: string, to: string) =>
    customerRequest<CustomerAvailability>(
      "POST",
      `/reservations/${uuid}/availability`,
      { body: { from, to } },
    ),
  reschedule: (uuid: string, startsAt: string, expectedVersion: number) =>
    customerRequest<CustomerReservation>(
      "POST",
      `/reservations/${uuid}/reschedule`,
      {
        body: { starts_at: startsAt, expected_version: expectedVersion },
        idempotent: true,
      },
    ),
  payments: (params: CustomerListParams) =>
    customerRequest<CustomerReservationPage>("GET", "/payments", {
      query: params,
    }),
  paymentSummary: () =>
    customerRequest<CustomerPaymentSummary>("GET", "/payments/summary"),
  sessions: () => customerRequest<CustomerDeviceList>("GET", "/sessions"),
  revokeSession: (uuid: string) =>
    customerRequest<void>("DELETE", `/sessions/${uuid}`),
  startPhoneChange: (phone: string) =>
    customerRequest<CustomerSignInStart>("POST", "/phone/start", {
      body: { phone },
    }),
  verifyPhoneChange: (phone: string, code: string) =>
    customerRequest<CustomerProfile>("POST", "/phone/verify", {
      body: { phone, code },
    }),
  reportReassigned: (keepBusinesses: string[]) =>
    customerRequest<CustomerProfile>("POST", "/phone/reassigned", {
      body: { keep_businesses: keepBusinesses },
    }),
  /** Visit link: the token is the only key; no session is involved. */
  visit: (token: string) =>
    customerRequest<CustomerReservation>("POST", "/visit", {
      body: { token },
    }),
  visitCancel: (token: string, expectedVersion: number) =>
    customerRequest<CustomerReservation>("POST", "/visit/cancel", {
      body: { token, expected_version: expectedVersion },
    }),
  visitConfirm: (token: string) =>
    customerRequest<CustomerReservation>("POST", "/visit/confirm", {
      body: { token },
    }),
  visitRequestChange: (token: string, note: string) =>
    customerRequest<CustomerReservation>("POST", "/visit/change-request", {
      body: { token, note },
    }),
};

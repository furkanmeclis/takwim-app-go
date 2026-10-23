import { cookies } from "next/headers";

import {
  clientIpFromHeaders,
  fetchUpstream,
  type UpstreamResult,
} from "@/lib/server/upstream";

/**
 * Customer panel session (takwim.link/panel, `/v1/customer/*`).
 *
 * The API answers a verified sign-in with a customer token (kind
 * `customer`). The BFF keeps it in its own httpOnly cookie, scoped to the
 * customer API paths, and sends it upstream as Bearer. The staff session
 * (NextAuth cookie) is never forwarded on these paths and the customer
 * cookie never reaches any other path, so the two sessions cannot mix.
 */
const COOKIE_PATH = "/api/v1/customer";
const VERIFY_PATH = "customer/auth/verify";
const LOGOUT_PATH = "customer/auth/logout";

export function customerCookieName() {
  return process.env.NODE_ENV === "production"
    ? "__Secure-takwim-customer"
    : "takwim-customer";
}

function cookieOptions(maxAge: number) {
  return {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: COOKIE_PATH,
    maxAge,
  };
}

export function isCustomerPath(segments: string[]) {
  return segments[0] === "customer";
}

type VerifyEnvelope = {
  success?: boolean;
  data?: {
    access_token?: string;
    token_type?: string;
    expires_at?: string;
  } & Record<string, unknown>;
} & Record<string, unknown>;

function parse(buffer: ArrayBuffer): VerifyEnvelope | null {
  try {
    const text = new TextDecoder().decode(buffer);
    return text ? (JSON.parse(text) as VerifyEnvelope) : null;
  } catch {
    return null;
  }
}

const FORWARDED_RESPONSE_HEADERS = [
  "content-type",
  "cache-control",
  "x-request-id",
  "idempotent-replayed",
  "retry-after",
] as const;

function responseHeaders(upstream: Headers): Headers {
  const out = new Headers();
  for (const key of FORWARDED_RESPONSE_HEADERS) {
    const value = upstream.get(key);
    if (value) out.set(key, value);
  }
  out.set("Cache-Control", "no-store");
  return out;
}

/** Proxies one `/api/v1/customer/*` request with the customer cookie. */
export async function proxyCustomer(
  path: string,
  pathWithQuery: string,
  request: Request,
  body: ArrayBuffer,
): Promise<Response> {
  const jar = await cookies();
  const name = customerCookieName();
  const token = jar.get(name)?.value;

  const headers = new Headers({ Accept: "application/json" });
  const contentType = request.headers.get("content-type");
  if (contentType) headers.set("Content-Type", contentType);
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) headers.set("X-Forwarded-For", clientIp);
  const userAgent = request.headers.get("user-agent");
  if (userAgent && path === VERIFY_PATH) headers.set("User-Agent", userAgent);
  const idempotencyKey = request.headers.get("idempotency-key")?.trim();
  if (idempotencyKey && request.method !== "GET") {
    headers.set("Idempotency-Key", idempotencyKey);
  }
  if (token && path !== VERIFY_PATH) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const result: UpstreamResult = await fetchUpstream(pathWithQuery, {
    method: request.method,
    headers,
    body: body.byteLength > 0 ? body : null,
  });

  let responseBody: ArrayBuffer | string = result.body;
  if (path === VERIFY_PATH && result.status === 200) {
    const envelope = parse(result.body);
    const access = envelope?.data?.access_token;
    if (envelope?.data && access) {
      const expires = Date.parse(envelope.data.expires_at ?? "");
      const maxAge = Number.isFinite(expires)
        ? Math.max(60, Math.floor((expires - Date.now()) / 1000))
        : 60 * 60 * 24;
      jar.set(name, access, cookieOptions(maxAge));
      const data = { ...envelope.data, authenticated: true };
      delete data.access_token;
      delete data.token_type;
      responseBody = JSON.stringify({ ...envelope, data });
    }
  }
  if (path === LOGOUT_PATH || result.status === 401) {
    jar.set(name, "", cookieOptions(0));
  }

  const empty = result.status === 204 || result.status === 304;
  return new Response(empty ? null : responseBody, {
    status: result.status,
    headers: responseHeaders(result.headers),
  });
}

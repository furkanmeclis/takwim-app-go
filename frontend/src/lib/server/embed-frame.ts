import { NextResponse, type NextRequest } from "next/server";

import { clientIpFromHeaders, fetchUpstream } from "@/lib/server/upstream";

/**
 * Framing policy of the embedded booking screen (TEC-908). Every other
 * page is `frame-ancestors 'self'` (next.config.ts); `/embed/{slug}/…` may
 * also be framed by the sites in the business's embed settings, read from
 * the public profile (`embed.allowed_origins`). The answer is cached per
 * business for a minute; when the profile cannot be read the page falls
 * back to `'self'` only (fail closed).
 */

const EMBED_PATH_RE = /^\/embed\/([^/]+)(?:\/|$)/;
/** What a CSP source may look like; anything else is dropped. */
const SOURCE_RE = /^https?:\/\/(\*\.)?[a-z0-9.-]+(:\d{1,5})?$/;
const TTL_MS = 60_000;
const FAILURE_TTL_MS = 10_000;
const MAX_ENTRIES = 1000;

type Entry = { origins: string[]; expires: number };
const cache = new Map<string, Entry>();

export function embedSlug(pathname: string): string | null {
  const match = EMBED_PATH_RE.exec(pathname);
  if (!match) return null;
  try {
    return decodeURIComponent(match[1]!).toLowerCase();
  } catch {
    return null;
  }
}

async function allowedOrigins(
  slug: string,
  request: NextRequest,
): Promise<string[]> {
  const now = Date.now();
  const hit = cache.get(slug);
  if (hit && hit.expires > now) return hit.origins;

  let origins: string[] = [];
  let ttl = TTL_MS;
  try {
    const headers = new Headers({ Accept: "application/json" });
    const ip = clientIpFromHeaders(request.headers);
    if (ip) headers.set("X-Forwarded-For", ip);
    const result = await fetchUpstream(
      `public/businesses/${encodeURIComponent(slug)}`,
      { method: "GET", headers },
    );
    if (result.status === 200) {
      const payload = JSON.parse(new TextDecoder().decode(result.body)) as {
        data?: { embed?: { allowed_origins?: unknown } };
      };
      const list = payload.data?.embed?.allowed_origins;
      origins = Array.isArray(list)
        ? list.filter(
            (o): o is string => typeof o === "string" && SOURCE_RE.test(o),
          )
        : [];
    } else if (result.status !== 404) {
      ttl = FAILURE_TTL_MS;
    }
  } catch {
    ttl = FAILURE_TTL_MS;
  }

  if (cache.size >= MAX_ENTRIES) cache.clear();
  cache.set(slug, { origins, expires: now + ttl });
  return origins;
}

/** `frame-ancestors` of one business's embedded screen. */
export function embedFramePolicy(origins: string[]) {
  return ["frame-ancestors 'self'", ...origins].join(" ") + "; base-uri 'self'";
}

/** Proxy answer of an `/embed/…` page: the request goes on, framable only by the business's sites. */
export async function embedFrameResponse(
  request: NextRequest,
  slug: string,
): Promise<NextResponse> {
  const origins = await allowedOrigins(slug, request);
  const response = NextResponse.next();
  response.headers.set("Content-Security-Policy", embedFramePolicy(origins));
  return response;
}

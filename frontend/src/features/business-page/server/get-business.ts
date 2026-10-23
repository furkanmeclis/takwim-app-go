import { headers } from "next/headers";
import { cache } from "react";

import type { PublicBusiness } from "@/features/business-page/types";
import { clientIpFromHeaders, fetchUpstream } from "@/lib/server/upstream";

export type BusinessLookup =
  | { status: "found"; business: PublicBusiness }
  | { status: "not_found" }
  /** API down or rate limited: the client page asks again. */
  | { status: "unavailable" };

/**
 * Profile of a business for the server render (SEO metadata and the first
 * paint). One call per request: metadata and page share it. The visitor's
 * IP goes along so the API's per-IP limit counts the visitor, not the app.
 */
export const getPublicBusiness = cache(
  async (slug: string): Promise<BusinessLookup> => {
    const incoming = await headers();
    const forwarded = new Headers({ Accept: "application/json" });
    const ip = clientIpFromHeaders(incoming);
    if (ip) forwarded.set("X-Forwarded-For", ip);
    try {
      const result = await fetchUpstream(
        `public/businesses/${encodeURIComponent(slug)}`,
        { method: "GET", headers: forwarded },
      );
      if (result.status === 404) return { status: "not_found" };
      if (result.status !== 200) return { status: "unavailable" };
      const payload = JSON.parse(new TextDecoder().decode(result.body)) as {
        data?: PublicBusiness;
      };
      return payload.data
        ? { status: "found", business: payload.data }
        : { status: "unavailable" };
    } catch {
      return { status: "unavailable" };
    }
  },
);

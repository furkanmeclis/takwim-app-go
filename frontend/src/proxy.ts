import type { NextFetchEvent, NextRequest } from "next/server";

import { auth } from "@/auth";
import { embedFrameResponse, embedSlug } from "@/lib/server/embed-frame";

// NextAuth's `auth` doubles as the request proxy (its `authorized` callback
// guards the panel routes).
const authProxy = auth as unknown as (
  request: NextRequest,
  event: NextFetchEvent,
) => Promise<Response | undefined>;

/**
 * Panel routes go through the auth guard. The embedded booking screen
 * (`/embed/{slug}/…`, TEC-908) is public and only gets its framing policy:
 * the business's allowed sites in `frame-ancestors`.
 */
export async function proxy(request: NextRequest, event: NextFetchEvent) {
  const slug = embedSlug(request.nextUrl.pathname);
  if (slug) return embedFrameResponse(request, slug);
  return authProxy(request, event);
}

export const config = {
  matcher: [
    "/",
    "/profile/:path*",
    "/organizations",
    "/no-access",
    "/platform/:path*",
    "/t/:path*",
    "/embed/:path*",
  ],
};

import * as Sentry from "@sentry/nextjs";

import { proxyToUpstream } from "@/lib/server/bff-proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Long-lived SSE streams need a raised
 * function timeout. Align with API `HTTP_WRITE_TIMEOUT` (e.g. 120s).
 */
export const maxDuration = 120;

async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  try {
    return await proxyToUpstream(path ?? [], request);
  } catch (err) {
    if (request.signal.aborted) {
      // Client went away mid-request: not a server fault.
      return new Response(null, { status: 499 });
    }
    console.error("[api/v1]", (path ?? []).join("/"), err);
    Sentry.captureException(err, {
      tags: {
        "http.route": "/api/v1/[...path]",
        "http.method": request.method,
      },
      fingerprint: ["bff-proxy", request.method],
    });
    return new Response(
      JSON.stringify({ success: false, error: "internal_error" }),
      {
        status: 500,
        headers: { "Content-Type": "application/json" },
      },
    );
  }
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
export const OPTIONS = handle;

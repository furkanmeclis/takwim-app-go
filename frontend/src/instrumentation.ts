import * as Sentry from "@sentry/nextjs";

export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    await import("./sentry.server.config");
  }
  if (process.env.NEXT_RUNTIME === "edge") {
    await import("./sentry.edge.config");
  }
}

// Server component, route handler, proxy and server action errors. A no-op
// when Sentry was not initialised (no SENTRY_DSN).
export const onRequestError = Sentry.captureRequestError;

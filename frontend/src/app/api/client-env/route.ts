/**
 * Runtime browser config as a tiny script (`window.__APP_ENV__ = {...}`).
 * Loaded before hydration by the root layout so instrumentation-client.ts can
 * read the Sentry DSN from the container env (SENTRY_DSN) instead of a
 * build-time NEXT_PUBLIC_* variable. A DSN is public by design.
 */
export const dynamic = "force-dynamic";

export function GET() {
  const payload = {
    sentryDsn: process.env.SENTRY_DSN?.trim() || "",
    sentryEnvironment:
      process.env.SENTRY_ENVIRONMENT?.trim() || process.env.NODE_ENV || "",
  };
  const json = JSON.stringify(payload).replace(/</g, "\\u003c");
  // instrumentation-client.ts registers __onAppEnv before this runs.
  const body = `window.__APP_ENV__=${json};window.__onAppEnv&&window.__onAppEnv(window.__APP_ENV__);`;
  return new Response(body, {
    headers: {
      "Content-Type": "application/javascript; charset=utf-8",
      "Cache-Control": "no-store",
    },
  });
}

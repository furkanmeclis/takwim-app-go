import * as Sentry from "@sentry/nextjs";

import { commonSentryOptions } from "@/lib/observability/sentry-options";

/**
 * The browser DSN is delivered at runtime by /api/client-env (root layout,
 * strategy="beforeInteractive"), because the image is built without any
 * Sentry env and NEXT_PUBLIC_* would be inlined at build time.
 *
 * Next runs this file before it loads beforeInteractive scripts (both happen
 * before hydration), so the script calls back into __onAppEnv; if it already
 * ran (order is not guaranteed across Next versions) we init right away.
 */
type ClientEnv = { sentryDsn?: string; sentryEnvironment?: string };
type AppEnvGlobal = {
  __APP_ENV__?: ClientEnv;
  __onAppEnv?: (env: ClientEnv) => void;
};

let initialised = false;

function initSentry(env: ClientEnv | undefined) {
  if (initialised || !env?.sentryDsn) return;
  initialised = true;
  Sentry.init({
    ...commonSentryOptions,
    dsn: env.sentryDsn,
    environment: env.sentryEnvironment || "production",
  });
}

const g = globalThis as AppEnvGlobal;
g.__onAppEnv = initSentry;
initSentry(g.__APP_ENV__);

export const onRouterTransitionStart = Sentry.captureRouterTransitionStart;

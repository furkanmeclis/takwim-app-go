import * as Sentry from "@sentry/nextjs";

import {
  commonSentryOptions,
  serverSentryConfig,
} from "@/lib/observability/sentry-options";

const config = serverSentryConfig();

if (config) {
  Sentry.init({
    ...commonSentryOptions,
    ...config,
  });
}

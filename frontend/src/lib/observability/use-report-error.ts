"use client";

import * as Sentry from "@sentry/nextjs";
import { useEffect } from "react";

/** Reports an error caught by a route error boundary (no-op without a DSN). */
export function useReportError(error: Error & { digest?: string }) {
  useEffect(() => {
    Sentry.captureException(error);
  }, [error]);
}

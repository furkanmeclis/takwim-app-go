import * as Sentry from "@sentry/nextjs";

import { scrubString } from "@/lib/observability/sentry-options";

/**
 * NextAuth error types caused by the user or the browser (wrong password,
 * denied consent, stale tab). Logged, not reported.
 */
const EXPECTED_AUTH_ERRORS = new Set([
  "CredentialsSignin",
  "AccessDenied",
  "Verification",
  "OAuthAccountNotLinked",
  "MissingCSRF",
  "UnknownAction",
]);

type AuthLikeError = Error & { type?: string; cause?: unknown };

function causeMessage(cause: unknown): string | undefined {
  if (!cause || typeof cause !== "object") return undefined;
  const err = (cause as { err?: unknown }).err;
  if (err instanceof Error) return err.message;
  return undefined;
}

/**
 * NextAuth `logger.error` hook. Replaces the default `[auth][error]` console
 * output (same prefix, scrubbed) and reports unexpected errors — sign-in
 * callback failures, adapter/backend errors, JWT/session decode errors.
 */
export function logAuthError(error: Error) {
  const err = error as AuthLikeError;
  const type = err.type || err.name || "AuthError";
  const cause = causeMessage(err.cause);
  console.error(
    `[auth][error] ${type}: ${scrubString(err.message ?? "")}` +
      (cause ? ` | cause: ${scrubString(cause)}` : ""),
  );
  if (EXPECTED_AUTH_ERRORS.has(type)) return;
  Sentry.captureException(error, {
    level: "error",
    tags: { "auth.error_type": type },
    fingerprint: ["nextauth", type],
  });
}

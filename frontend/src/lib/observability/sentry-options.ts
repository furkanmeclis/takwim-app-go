/**
 * Shared Sentry options for the browser, Node.js server and edge runtimes.
 *
 * Privacy: no request bodies, cookies, query strings or auth headers are sent;
 * free text is scrubbed of emails, phone numbers and tokens; the user is
 * identified by the internal user id only. Everything is off when no DSN is
 * configured (SENTRY_DSN, read at runtime).
 */
import type { ErrorEvent, EventHint } from "@sentry/nextjs";

const EMAIL = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g;
const PUSH_TOKEN = /(?:Exponent|Expo)PushToken(?:\[|%5B)[^\]%\s]+(?:\]|%5D)?/gi;
const JWT = /eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*/g;
const BEARER = /\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]+/gi;
const SECRET_PAIR =
  /\b((?:access_|refresh_|id_|link_)?token|password|passwd|secret|api[_-]?key|authorization|cookie|code|otp|ticket|dsn|csrfToken)(\s*=\s*|"\s*:\s*"?)([^\s"'&,;]+)/gi;
const PHONE_INTL = /\+\d{1,3}[\s.-]?\(?\d{2,4}\)?(?:[\s.-]?\d{2,4}){2,4}/g;
const PHONE_TR = /\b0?5\d{2}[\s.-]?\d{3}[\s.-]?\d{2}[\s.-]?\d{2}\b/g;

const SENSITIVE_KEY =
  /password|passwd|secret|token|authorization|cookie|email|phone|gsm|dsn|api_?key|private_key|otp|ticket|credential|session/i;

const ALLOWED_HEADERS = new Set([
  "user-agent",
  "content-type",
  "content-length",
  "accept",
  "x-request-id",
]);

export function scrubString(value: string): string {
  if (!value) return value;
  return value
    .replace(PUSH_TOKEN, "[push-token]")
    .replace(JWT, "[token]")
    .replace(BEARER, "$1 [token]")
    .replace(SECRET_PAIR, "$1$2[redacted]")
    .replace(EMAIL, "[email]")
    .replace(PHONE_INTL, "[phone]")
    .replace(PHONE_TR, "[phone]");
}

/** Drops query string and fragment (they can carry emails / tokens). */
export function scrubUrl(value: string): string {
  if (!value) return value;
  const cut = value.search(/[?#]/);
  return scrubString(cut === -1 ? value : value.slice(0, cut));
}

export function scrubValue(value: unknown, key = "", depth = 0): unknown {
  if (key && SENSITIVE_KEY.test(key)) return "[redacted]";
  if (typeof value === "string") return scrubString(value);
  if (value === null || typeof value !== "object" || depth > 6) return value;
  if (Array.isArray(value))
    return value.map((v) => scrubValue(v, "", depth + 1));
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    out[k] = scrubValue(v, k, depth + 1);
  }
  return out;
}

export function scrubEvent<T extends ErrorEvent>(event: T): T {
  if (event.user) {
    event.user = event.user.id ? { id: event.user.id } : {};
  }
  if (event.request) {
    const req = event.request;
    delete req.cookies;
    delete req.data;
    delete req.query_string;
    delete req.env;
    if (req.url) req.url = scrubUrl(req.url);
    if (req.headers) {
      const kept: Record<string, string> = {};
      for (const [k, v] of Object.entries(req.headers)) {
        if (ALLOWED_HEADERS.has(k.toLowerCase()))
          kept[k] = scrubString(String(v));
      }
      req.headers = kept;
    }
  }
  if (event.message) event.message = scrubString(event.message);
  if (event.transaction) event.transaction = scrubUrl(event.transaction);
  for (const ex of event.exception?.values ?? []) {
    if (ex.value) ex.value = scrubString(ex.value);
  }
  if (event.extra) event.extra = scrubValue(event.extra) as typeof event.extra;
  if (event.contexts) {
    event.contexts = scrubValue(event.contexts) as typeof event.contexts;
  }
  if (event.tags) event.tags = scrubValue(event.tags) as typeof event.tags;
  for (const crumb of event.breadcrumbs ?? []) {
    if (crumb.message) crumb.message = scrubString(crumb.message);
    if (crumb.data) {
      const data = scrubValue(crumb.data) as Record<string, unknown>;
      if (typeof data.url === "string") data.url = scrubUrl(data.url);
      if (typeof data.to === "string") data.to = scrubUrl(data.to);
      if (typeof data.from === "string") data.from = scrubUrl(data.from);
      crumb.data = data;
    }
  }
  return event;
}

export function beforeSend(event: ErrorEvent, _hint: EventHint): ErrorEvent {
  return scrubEvent(event);
}

/** Runtime config for server and edge (never inlined at build time). */
export function serverSentryConfig(): {
  dsn: string;
  environment: string;
} | null {
  const dsn = process.env.SENTRY_DSN?.trim();
  if (!dsn) return null;
  return {
    dsn,
    environment:
      process.env.SENTRY_ENVIRONMENT?.trim() ||
      process.env.NODE_ENV ||
      "production",
  };
}

export const commonSentryOptions = {
  sendDefaultPii: false,
  // Tracing stays off unless a sample rate is set (undefined = no spans).
  tracesSampleRate: undefined,
  maxBreadcrumbs: 30,
  beforeSend,
} as const;

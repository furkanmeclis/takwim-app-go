/**
 * Embedded booking screen (TEC-908): pre-selection from the URL, the
 * message protocol between the frame and the loader script
 * (`public/embed.js`), and the parent's origin check.
 *
 * Messages never carry customer data: only the frame id, the message type
 * and, for `resize`, the content height.
 */

/** `source` of every message the frame sends (the loader checks it). */
export const EMBED_MESSAGE_SOURCE = "takwim-embed";

/** Message protocol version (`v`). */
export const EMBED_MESSAGE_VERSION = 1;

export type EmbedMessageType = "ready" | "resize" | "scroll_top";

/** Base path of the embedded screens (`takwim.link/embed/{slug}`). */
export const EMBED_BASE = "/embed";

/** Base path of the standalone business page. */
export const PAGE_BASE = "/b";

/** Full view (header, profile, tabs) or the compact wizard only. */
export type EmbedView = "compact" | "full";

/**
 * Pre-selection of the booking screen. Every field is checked twice: the
 * server page keeps only well-formed values, and the wizard applies one
 * only when the API lists it for the business (a hidden or another
 * business's branch, service or person is ignored); the API checks the
 * booking itself again.
 */
export type Preselect = {
  branch?: string;
  service?: string;
  staff?: string;
  /** `YYYY-MM-DD` in the business's time zone. */
  date?: string;
};

/** Context of an embedded screen. */
export type EmbedContext = {
  /** Frame id the loader gave (`frame`), echoed in every message. */
  frame: string;
  view: EmbedView;
  /** The parent's origin as the loader saw it (`origin`); checked again. */
  parentOrigin: string;
  /** Opened in the loader's window (button): the frame scrolls itself. */
  modal: boolean;
};

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const FRAME_RE = /^[A-Za-z0-9_-]{1,32}$/;
const ORIGIN_RE = /^https?:\/\/(\*\.)?[a-z0-9.-]+(:\d{1,5})?$/;

type Params = Record<string, string | string[] | undefined>;

function one(params: Params, key: string) {
  const value = params[key];
  return typeof value === "string" ? value.trim() : undefined;
}

function validDate(value: string | undefined) {
  if (!value || !DATE_RE.test(value)) return undefined;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) &&
    parsed.toISOString().slice(0, 10) === value
    ? value
    : undefined;
}

/** Well-formed pre-selection of the URL (`branch`, `service`, `staff`, `date`). */
export function parsePreselect(params: Params): Preselect {
  const out: Preselect = {};
  for (const key of ["branch", "service", "staff"] as const) {
    const value = one(params, key);
    if (value && UUID_RE.test(value)) out[key] = value.toLowerCase();
  }
  const date = validDate(one(params, "date"));
  if (date) out.date = date;
  return out;
}

/** Embed context of the URL (`frame`, `view`, `origin`, `mode`). */
export function parseEmbedContext(params: Params): EmbedContext {
  const frame = one(params, "frame");
  const origin = one(params, "origin")?.toLowerCase();
  return {
    frame: frame && FRAME_RE.test(frame) ? frame : "",
    view: one(params, "view") === "full" ? "full" : "compact",
    parentOrigin: origin && ORIGIN_RE.test(origin) ? origin : "",
    modal: one(params, "mode") === "modal",
  };
}

/** Query string that keeps the embed context and the pre-selection. */
export function embedQuery(ctx: EmbedContext, preselect: Preselect = {}) {
  const query = new URLSearchParams();
  if (ctx.frame) query.set("frame", ctx.frame);
  if (ctx.view === "full") query.set("view", "full");
  if (ctx.parentOrigin) query.set("origin", ctx.parentOrigin);
  if (ctx.modal) query.set("mode", "modal");
  for (const [key, value] of Object.entries(preselect)) {
    if (value) query.set(key, value);
  }
  const text = query.toString();
  return text ? `?${text}` : "";
}

/**
 * Whether `origin` is one of the business's allowed sites. A `*.` entry
 * allows every subdomain (not the bare domain), as in a CSP source.
 */
export function originAllowed(origin: string, allowed: string[]) {
  const value = origin.toLowerCase();
  return allowed.some((entry) => {
    const rule = entry.toLowerCase();
    if (rule === value) return true;
    const star = rule.indexOf("://*.");
    if (star < 0) return false;
    const scheme = rule.slice(0, star + 3);
    const suffix = rule.slice(star + 4);
    return (
      value.startsWith(scheme) &&
      value.endsWith(suffix) &&
      value.length > scheme.length + suffix.length
    );
  });
}

/**
 * The parent page's origin the frame may talk to, or null. The browser's
 * own answer comes first (`location.ancestorOrigins`, else the referrer's
 * origin), then the loader's `origin`; it must be an allowed site or this
 * app itself (the panel's preview).
 */
export function resolveParentOrigin(
  ctx: EmbedContext,
  allowed: string[],
): string | null {
  if (typeof window === "undefined" || window.parent === window) return null;
  const candidates: string[] = [];
  const ancestors = window.location.ancestorOrigins;
  if (ancestors && ancestors.length > 0) {
    candidates.push(ancestors[0]!);
  } else {
    try {
      if (document.referrer) {
        candidates.push(new URL(document.referrer).origin);
      }
    } catch {
      // No usable referrer.
    }
    if (ctx.parentOrigin) candidates.push(ctx.parentOrigin);
  }
  for (const candidate of candidates) {
    const origin = candidate.toLowerCase();
    if (origin === window.location.origin || originAllowed(origin, allowed)) {
      return origin;
    }
  }
  return null;
}

/** Sends one message to the parent page (only to its checked origin). */
export function postToParent(
  target: string | null,
  frame: string,
  type: EmbedMessageType,
  extra: { height?: number } = {},
) {
  if (!target || !frame) return;
  window.parent.postMessage(
    {
      source: EMBED_MESSAGE_SOURCE,
      v: EMBED_MESSAGE_VERSION,
      frame,
      type,
      ...extra,
    },
    target,
  );
}

/** Whether this document runs inside a frame. */
export function inFrame() {
  try {
    return window.self !== window.top;
  } catch {
    return true;
  }
}

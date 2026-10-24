/**
 * Embed code of the booking screen (TEC-908): the snippet a business pastes
 * on its own site, built for `public/embed.js`.
 */

export type EmbedMode = "inline" | "button";

export type EmbedCodeOptions = {
  /** Origin that serves `/embed.js` and `/embed/{slug}` (this app). */
  origin: string;
  slug: string;
  mode: EmbedMode;
  view: "compact" | "full";
  branch?: string;
  service?: string;
  staff?: string;
  /** `YYYY-MM-DD`. */
  date?: string;
  /** Text of the button (button mode). */
  label: string;
};

function escapeHTML(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function dataAttributes(o: EmbedCodeOptions) {
  const out: string[] = [];
  if (o.view === "full") out.push(`data-takwim-view="full"`);
  for (const key of ["branch", "service", "staff", "date"] as const) {
    const value = o[key];
    if (value) out.push(`data-takwim-${key}="${escapeHTML(value)}"`);
  }
  return out.length ? ` ${out.join(" ")}` : "";
}

/** The HTML to paste: the element and the loader script. */
export function embedSnippet(o: EmbedCodeOptions) {
  const slug = escapeHTML(o.slug);
  const attrs = dataAttributes(o);
  const element =
    o.mode === "button"
      ? `<button type="button" data-takwim-open="${slug}"${attrs}>${escapeHTML(o.label.trim() || o.slug)}</button>`
      : `<div data-takwim-slug="${slug}"${attrs}></div>`;
  return `${element}\n<script async src="${o.origin}/embed.js"></script>`;
}

/** Query of the pre-selection (shared by the direct link and the preview). */
function preselectQuery(o: EmbedCodeOptions) {
  const query = new URLSearchParams();
  for (const key of ["branch", "service", "staff", "date"] as const) {
    const value = o[key];
    if (value) query.set(key, value);
  }
  return query;
}

/** Direct link to the business page with the same pre-selection. */
export function preselectLink(o: EmbedCodeOptions) {
  const query = preselectQuery(o).toString();
  return `${o.origin}/b/${encodeURIComponent(o.slug)}${query ? `?${query}` : ""}`;
}

/** Same-origin preview of the embedded screen (scrolls inside its frame). */
export function previewPath(o: EmbedCodeOptions) {
  const query = preselectQuery(o);
  query.set("mode", "modal");
  if (o.view === "full") query.set("view", "full");
  return `/embed/${encodeURIComponent(o.slug)}?${query.toString()}`;
}

/**
 * Access tokens of the bookings made on this device. The status link
 * carries the token in its fragment (`#t=…`), which the browser never sends
 * to a server or in a Referer; the same token is kept in localStorage so the
 * status page also opens from a plain link on the device that booked.
 */
const STORAGE_PREFIX = "takwim.booking.";
const TOKEN_RE = /^[0-9a-f]{64}$/;

/**
 * Status page of a booking: `/b/…` on the business page, `/embed/…` inside
 * the embedded screen (TEC-908), which keeps its query (frame id, view).
 */
export function statusPath(
  slug: string,
  uuid: string,
  base = "/b",
  query = "",
) {
  return `${base}/${encodeURIComponent(slug)}/reservations/${encodeURIComponent(uuid)}${query}`;
}

/**
 * Status link of a booking with its token in the fragment. The token
 * travels in the link, not in a cookie, so the embedded screen works with
 * third-party cookies and storage blocked (A21).
 */
export function statusHref(
  slug: string,
  uuid: string,
  token: string,
  base = "/b",
  query = "",
) {
  return `${statusPath(slug, uuid, base, query)}#t=${token}`;
}

export function rememberAccessToken(uuid: string, token: string) {
  try {
    window.localStorage.setItem(`${STORAGE_PREFIX}${uuid}`, token);
  } catch {
    // Private mode or storage switched off: the fragment link still works.
  }
}

/** Token from the URL fragment, else the one kept on this device. */
export function readAccessToken(uuid: string): string | null {
  const fromHash = new URLSearchParams(window.location.hash.slice(1)).get("t");
  if (fromHash && TOKEN_RE.test(fromHash)) {
    rememberAccessToken(uuid, fromHash);
    return fromHash;
  }
  try {
    const stored = window.localStorage.getItem(`${STORAGE_PREFIX}${uuid}`);
    return stored && TOKEN_RE.test(stored) ? stored : null;
  } catch {
    return null;
  }
}

/**
 * Branch picked in the tenant header, per organization. Every
 * `tenantRequest` sends it as `X-Branch` so branch-bound lists, counts,
 * search and the overview narrow to that branch (the API checks it against
 * the member's branch scope). No selection = all branches the member may
 * access. Kept in localStorage so a reload keeps the view; storage errors
 * (private mode, blocked site data) fall back to memory.
 */

/** Header that names the picked branch (branch uuid). */
export const BRANCH_HEADER = "X-Branch";

const STORAGE_PREFIX = "takwim.branch.";

const memory = new Map<string, string | null>();
const listeners = new Set<() => void>();

function read(slug: string): string | null {
  if (memory.has(slug)) return memory.get(slug) ?? null;
  let value: string | null = null;
  try {
    value = window.localStorage.getItem(STORAGE_PREFIX + slug);
  } catch {
    value = null;
  }
  memory.set(slug, value);
  return value;
}

/** The picked branch uuid of an organization, or null for all branches. */
export function getSelectedBranch(slug: string): string | null {
  if (typeof window === "undefined" || !slug) return null;
  return read(slug);
}

/** Picks a branch (uuid) or all branches (null) for an organization. */
export function setSelectedBranch(slug: string, uuid: string | null) {
  if (!slug || getSelectedBranch(slug) === uuid) return;
  memory.set(slug, uuid);
  try {
    if (uuid) window.localStorage.setItem(STORAGE_PREFIX + slug, uuid);
    else window.localStorage.removeItem(STORAGE_PREFIX + slug);
  } catch {
    // Memory still holds the selection for this tab.
  }
  for (const listener of listeners) listener();
}

/** useSyncExternalStore subscription. */
export function subscribeSelectedBranch(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

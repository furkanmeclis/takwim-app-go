"use client";

import { useCallback, useSyncExternalStore } from "react";

import {
  getSelectedBranch,
  setSelectedBranch,
  subscribeSelectedBranch,
} from "@/lib/api/branch-selection";

/**
 * The branch picked in the tenant header for organization `slug` (uuid, or
 * null for all branches) and a setter. Requests pick it up on their own
 * (`X-Branch`); components read it to label the current view.
 */
export function useSelectedBranch(slug: string) {
  const branch = useSyncExternalStore(
    subscribeSelectedBranch,
    () => getSelectedBranch(slug),
    () => null,
  );
  const select = useCallback(
    (uuid: string | null) => setSelectedBranch(slug, uuid),
    [slug],
  );
  return [branch, select] as const;
}

"use client";

import type { ColumnFiltersState } from "@tanstack/react-table";
import { useSearchParams } from "next/navigation";
import { useMemo } from "react";

export type UrlListDefaults = {
  /** Column filters named in the URL (`?status=open,in_progress`). */
  initialColumnFilters: ColumnFiltersState | undefined;
  /** `?sort=` when it names one of the allowed sort fields. */
  initialSort: string | undefined;
};

/**
 * Start values of a server list from the URL, so another page can link to
 * the list with filters applied (a "Tümünü gör" link on a dashboard).
 * Only the given column ids and sort fields are read; values are
 * comma-separated (`?status=active&branch=<uuid>&sort=-created_at`).
 * Read once: `useServerListState` uses them as initial state.
 */
export function useUrlListDefaults(
  filterIds: readonly string[],
  sortFields: readonly string[] = [],
): UrlListDefaults {
  const params = useSearchParams();
  const query = params.toString();
  return useMemo(() => {
    const search = new URLSearchParams(query);
    const filters: ColumnFiltersState = [];
    for (const id of filterIds) {
      const values = (search.get(id) ?? "")
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean);
      if (values.length) filters.push({ id, value: values });
    }
    const sort = search.get("sort") ?? "";
    const field = sort.replace(/^-/, "");
    return {
      initialColumnFilters: filters.length ? filters : undefined,
      initialSort: field && sortFields.includes(field) ? sort : undefined,
    };
    // The lists read these once; ids are static per page.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);
}

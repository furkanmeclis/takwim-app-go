/** Values of a column filter (faceted filters keep an array). */
export function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** Comma-separated query value, or nothing for an empty filter. */
export function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/** Faceted filter options of a facet list (`GET /v1/tenant/classes/facets`). */
export function facetOptions(items?: { uuid: string; name: string }[]) {
  return (items ?? []).map((f) => ({ value: f.uuid, label: f.name }));
}

/** "2026-06-01" plus `days` calendar days. */
export function addDays(day: string, days: number) {
  const [y, m, d] = day.split("-").map(Number);
  const next = new Date(Date.UTC(y, m - 1, d + days));
  return next.toISOString().slice(0, 10);
}

/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const financeKeys = {
  all: (slug: string) => ["tenant", slug, "finance"] as const,
  categories: (slug: string, params: Record<string, unknown>) =>
    [...financeKeys.all(slug), "categories", params] as const,
  entries: (slug: string) => [...financeKeys.all(slug), "entries"] as const,
  entryList: (slug: string, params: Record<string, unknown>) =>
    [...financeKeys.entries(slug), "list", params] as const,
  entriesMeta: (slug: string) =>
    [...financeKeys.entries(slug), "meta"] as const,
  facets: (slug: string) => [...financeKeys.entries(slug), "facets"] as const,
  entry: (slug: string, uuid: string) =>
    [...financeKeys.entries(slug), "detail", uuid] as const,
  activity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...financeKeys.entry(slug, uuid), "activity", params] as const,
  summary: (slug: string, params: Record<string, unknown>) =>
    [...financeKeys.all(slug), "summary", params] as const,
};

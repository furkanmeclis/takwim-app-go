/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const reservationsKeys = {
  all: (slug: string) => ["tenant", slug, "reservations"] as const,
  lists: (slug: string) => [...reservationsKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...reservationsKeys.lists(slug), params] as const,
  meta: (slug: string) => [...reservationsKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...reservationsKeys.all(slug), "facets"] as const,
  calendar: (slug: string, params: Record<string, unknown>) =>
    [...reservationsKeys.all(slug), "calendar", params] as const,
  detail: (slug: string, uuid: string) =>
    [...reservationsKeys.all(slug), "detail", uuid] as const,
  availability: (slug: string, body: Record<string, unknown>) =>
    ["tenant", slug, "availability", body] as const,
};

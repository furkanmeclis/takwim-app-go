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
  agenda: (slug: string, params: Record<string, unknown>) =>
    [...reservationsKeys.all(slug), "agenda", params] as const,
  detail: (slug: string, uuid: string) =>
    [...reservationsKeys.all(slug), "detail", uuid] as const,
  payments: (slug: string, uuid: string) =>
    [...reservationsKeys.all(slug), "payments", uuid] as const,
  /** Under `all`: any visit change refreshes the follow-up list too. */
  followUps: (slug: string) =>
    [...reservationsKeys.all(slug), "follow-ups"] as const,
  followUpList: (slug: string, params: Record<string, unknown>) =>
    [...reservationsKeys.followUps(slug), "list", params] as const,
  followUpMeta: (slug: string) =>
    [...reservationsKeys.followUps(slug), "meta"] as const,
  followUpFacets: (slug: string, params: Record<string, unknown>) =>
    [...reservationsKeys.followUps(slug), "facets", params] as const,
  availability: (slug: string, body: Record<string, unknown>) =>
    ["tenant", slug, "availability", body] as const,
};

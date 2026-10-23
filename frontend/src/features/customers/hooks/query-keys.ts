/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const customersKeys = {
  all: (slug: string) => ["tenant", slug, "customers"] as const,
  lists: (slug: string) => [...customersKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...customersKeys.lists(slug), params] as const,
  meta: (slug: string) => [...customersKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...customersKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...customersKeys.all(slug), "detail", uuid] as const,
  activity: (slug: string, uuid: string) =>
    [...customersKeys.detail(slug, uuid), "activity"] as const,
};

/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const servicesKeys = {
  all: (slug: string) => ["tenant", slug, "services"] as const,
  lists: (slug: string) => [...servicesKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...servicesKeys.lists(slug), params] as const,
  meta: (slug: string) => [...servicesKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...servicesKeys.all(slug), "facets"] as const,
  categories: (slug: string) =>
    [...servicesKeys.all(slug), "categories"] as const,
  detail: (slug: string, uuid: string) =>
    [...servicesKeys.all(slug), "detail", uuid] as const,
  activity: (slug: string, uuid: string) =>
    [...servicesKeys.detail(slug, uuid), "activity"] as const,
};

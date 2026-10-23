/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const resourcesKeys = {
  all: (slug: string) => ["tenant", slug, "resources"] as const,
  lists: (slug: string) => [...resourcesKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...resourcesKeys.lists(slug), params] as const,
  meta: (slug: string) => [...resourcesKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...resourcesKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...resourcesKeys.all(slug), "detail", uuid] as const,
  closures: (slug: string, uuid: string) =>
    [...resourcesKeys.detail(slug, uuid), "closures"] as const,
  activity: (slug: string, uuid: string) =>
    [...resourcesKeys.detail(slug, uuid), "activity"] as const,
};

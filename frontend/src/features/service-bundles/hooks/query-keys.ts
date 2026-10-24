/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const serviceBundlesKeys = {
  all: (slug: string) => ["tenant", slug, "service-bundles"] as const,
  lists: (slug: string) => [...serviceBundlesKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...serviceBundlesKeys.lists(slug), params] as const,
  meta: (slug: string) => [...serviceBundlesKeys.all(slug), "meta"] as const,
  facets: (slug: string) =>
    [...serviceBundlesKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...serviceBundlesKeys.all(slug), "detail", uuid] as const,
  activity: (slug: string, uuid: string) =>
    [...serviceBundlesKeys.detail(slug, uuid), "activity"] as const,
};

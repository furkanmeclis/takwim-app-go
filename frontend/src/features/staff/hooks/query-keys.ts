/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const staffKeys = {
  all: (slug: string) => ["tenant", slug, "staff"] as const,
  lists: (slug: string) => [...staffKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...staffKeys.lists(slug), params] as const,
  meta: (slug: string) => [...staffKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...staffKeys.all(slug), "facets"] as const,
  linkable: (slug: string) => [...staffKeys.all(slug), "linkable"] as const,
  detail: (slug: string, uuid: string) =>
    [...staffKeys.all(slug), "detail", uuid] as const,
  timeOff: (slug: string, uuid: string) =>
    [...staffKeys.detail(slug, uuid), "time-off"] as const,
  activity: (slug: string, uuid: string) =>
    [...staffKeys.detail(slug, uuid), "activity"] as const,
};

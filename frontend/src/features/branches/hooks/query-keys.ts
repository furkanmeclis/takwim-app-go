/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const branchesKeys = {
  all: (slug: string) => ["tenant", slug, "branches"] as const,
  lists: (slug: string) => [...branchesKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...branchesKeys.lists(slug), params] as const,
  meta: (slug: string) => [...branchesKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...branchesKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...branchesKeys.all(slug), "detail", uuid] as const,
  servicePoints: (slug: string, uuid: string) =>
    [...branchesKeys.detail(slug, uuid), "service-points"] as const,
  activity: (slug: string, uuid: string) =>
    [...branchesKeys.detail(slug, uuid), "activity"] as const,
};

/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const productsKeys = {
  all: (slug: string) => ["tenant", slug, "products"] as const,
  lists: (slug: string) => [...productsKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...productsKeys.lists(slug), params] as const,
  meta: (slug: string) => [...productsKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...productsKeys.all(slug), "facets"] as const,
  lookup: (slug: string, code: string) =>
    [...productsKeys.all(slug), "lookup", code] as const,
  detail: (slug: string, uuid: string) =>
    [...productsKeys.all(slug), "detail", uuid] as const,
  activity: (slug: string, uuid: string) =>
    [...productsKeys.detail(slug, uuid), "activity"] as const,
  /** Movements sit under products: recording one refreshes both. */
  movements: (slug: string) =>
    [...productsKeys.all(slug), "movements"] as const,
  movementList: (slug: string, params: Record<string, unknown>) =>
    [...productsKeys.movements(slug), "list", params] as const,
  movementMeta: (slug: string) =>
    [...productsKeys.movements(slug), "meta"] as const,
  movementFacets: (slug: string) =>
    [...productsKeys.movements(slug), "facets"] as const,
};

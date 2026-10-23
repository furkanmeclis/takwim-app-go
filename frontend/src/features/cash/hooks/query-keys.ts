/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const cashKeys = {
  all: (slug: string) => ["tenant", slug, "cash"] as const,
  accounts: (slug: string) => [...cashKeys.all(slug), "accounts"] as const,
  accountList: (slug: string, params: Record<string, unknown>) =>
    [...cashKeys.accounts(slug), "list", params] as const,
  accountsMeta: (slug: string) => [...cashKeys.accounts(slug), "meta"] as const,
  accountFacets: (slug: string) =>
    [...cashKeys.accounts(slug), "facets"] as const,
  accountOptions: (slug: string) =>
    [...cashKeys.accounts(slug), "options"] as const,
  account: (slug: string, uuid: string) =>
    [...cashKeys.accounts(slug), "detail", uuid] as const,
  accountMovements: (
    slug: string,
    uuid: string,
    params: Record<string, unknown>,
  ) => [...cashKeys.account(slug, uuid), "movements", params] as const,
  closings: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...cashKeys.account(slug, uuid), "closings", params] as const,
  closingPreview: (slug: string, uuid: string, date: string) =>
    [...cashKeys.account(slug, uuid), "closing-preview", date] as const,
  activity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...cashKeys.account(slug, uuid), "activity", params] as const,
  movements: (slug: string) => [...cashKeys.all(slug), "movements"] as const,
  movementList: (slug: string, params: Record<string, unknown>) =>
    [...cashKeys.movements(slug), "list", params] as const,
  movementsMeta: (slug: string) =>
    [...cashKeys.movements(slug), "meta"] as const,
  movementFacets: (slug: string) =>
    [...cashKeys.movements(slug), "facets"] as const,
  daily: (slug: string, params: Record<string, unknown>) =>
    [...cashKeys.all(slug), "daily", params] as const,
};

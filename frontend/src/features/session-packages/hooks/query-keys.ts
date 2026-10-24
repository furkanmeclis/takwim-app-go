/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const sessionPackagesKeys = {
  all: (slug: string) => ["tenant", slug, "session-packages"] as const,
  packages: (slug: string, params: Record<string, unknown>) =>
    [...sessionPackagesKeys.all(slug), "packages", params] as const,
  packagesMeta: (slug: string) =>
    [...sessionPackagesKeys.all(slug), "packages-meta"] as const,
  sold: (slug: string, params: Record<string, unknown>) =>
    [...sessionPackagesKeys.all(slug), "sold", params] as const,
  soldDetail: (slug: string, uuid: string) =>
    [...sessionPackagesKeys.all(slug), "sold-detail", uuid] as const,
  soldFacets: (slug: string) =>
    [...sessionPackagesKeys.all(slug), "sold-facets"] as const,
  summary: (slug: string, params: Record<string, unknown>) =>
    [...sessionPackagesKeys.all(slug), "summary", params] as const,
};

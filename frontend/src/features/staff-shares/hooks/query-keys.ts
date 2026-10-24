/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const staffSharesKeys = {
  all: (slug: string) => ["tenant", slug, "staff-shares"] as const,
  summary: (slug: string, params: Record<string, unknown>) =>
    [...staffSharesKeys.all(slug), "summary", params] as const,
  mine: (slug: string, params: Record<string, unknown>) =>
    [...staffSharesKeys.all(slug), "me", params] as const,
  rules: (slug: string, params: Record<string, unknown>) =>
    [...staffSharesKeys.all(slug), "rules", params] as const,
  lines: (slug: string, params: Record<string, unknown>) =>
    [...staffSharesKeys.all(slug), "lines", params] as const,
  linesMeta: (slug: string) =>
    [...staffSharesKeys.all(slug), "lines-meta"] as const,
  payouts: (slug: string, params: Record<string, unknown>) =>
    [...staffSharesKeys.all(slug), "payouts", params] as const,
  payoutsMeta: (slug: string) =>
    [...staffSharesKeys.all(slug), "payouts-meta"] as const,
};

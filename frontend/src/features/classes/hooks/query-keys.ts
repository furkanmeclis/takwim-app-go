/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const classesKeys = {
  all: (slug: string) => ["tenant", slug, "classes"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...classesKeys.all(slug), "list", params] as const,
  facets: (slug: string) => [...classesKeys.all(slug), "facets"] as const,
  form: (slug: string) => [...classesKeys.all(slug), "form"] as const,
  sessions: (slug: string, params: Record<string, unknown>) =>
    [...classesKeys.all(slug), "sessions", params] as const,
  session: (slug: string, uuid: string, date: string) =>
    [...classesKeys.all(slug), "session", uuid, date] as const,
};

import { customersKeys } from "@/features/customers/hooks/query-keys";
import { reservationsKeys } from "@/features/reservations/hooks/query-keys";

/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const salesKeys = {
  all: (slug: string) => ["tenant", slug, "sales"] as const,
  lines: (slug: string) => [...salesKeys.all(slug), "lines"] as const,
  lineList: (slug: string, params: Record<string, unknown>) =>
    [...salesKeys.lines(slug), "list", params] as const,
  linesMeta: (slug: string) => [...salesKeys.lines(slug), "meta"] as const,
  line: (slug: string, uuid: string) =>
    [...salesKeys.lines(slug), "detail", uuid] as const,
  entries: (slug: string) => [...salesKeys.all(slug), "entries"] as const,
  entryList: (slug: string, params: Record<string, unknown>) =>
    [...salesKeys.entries(slug), "list", params] as const,
  entriesMeta: (slug: string) => [...salesKeys.entries(slug), "meta"] as const,
  summary: (slug: string, params: Record<string, unknown>) =>
    [...salesKeys.all(slug), "summary", params] as const,
  /** Under the reservation keys: every visit change (payment, completion) refreshes it. */
  reservationBalance: (slug: string, uuid: string) =>
    [...reservationsKeys.all(slug), "sales-balance", uuid] as const,
  /** Under the customer detail keys. */
  customerBalance: (slug: string, uuid: string) =>
    [...customersKeys.detail(slug, uuid), "sales-balance"] as const,
};

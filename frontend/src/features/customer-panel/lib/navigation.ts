import { routes } from "@/config/routes";

/** Only panel pages are valid return targets after sign-in. */
export function safeNext(next: string | null | undefined) {
  if (!next) return routes.customer.root;
  if (next !== routes.customer.root && !next.startsWith("/panel/")) {
    return routes.customer.root;
  }
  if (next.startsWith(routes.customer.login) || next.includes("//")) {
    return routes.customer.root;
  }
  return next;
}

export function loginHref(from: string) {
  const next = safeNext(from);
  return next === routes.customer.root
    ? routes.customer.login
    : `${routes.customer.login}?next=${encodeURIComponent(next)}`;
}

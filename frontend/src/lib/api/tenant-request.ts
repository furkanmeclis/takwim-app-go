import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api/client";
import { withStepUpRetry } from "@/features/step-up-engine/lib/step-up-interceptor";

/** Header that names the organization of a `/v1/tenant/*` request. */
export const ORGANIZATION_HEADER = "X-Organization";

const TENANT_PATH_PREFIX = "/v1/tenant/";

type TenantRequestOptions = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
  /** Skip the global error toast (callers render the error themselves). */
  silent?: boolean;
  signal?: AbortSignal;
};

/**
 * BFF fetch for organization-scoped endpoints. `slug` is the organization of
 * the current `/t/{slug}` route; it is sent as `X-Organization` so every
 * request names its organization and tabs on different organizations stay
 * independent. Only `/v1/tenant/*` paths are accepted: the BFF forwards the
 * header on those paths only.
 */
export async function tenantRequest<T>(
  slug: string,
  method: string,
  path: string,
  options?: TenantRequestOptions,
): Promise<T> {
  const pathname = path.startsWith("/") ? path : `/${path}`;
  if (!pathname.startsWith(TENANT_PATH_PREFIX)) {
    throw new Error(`tenantRequest: ${pathname} is not a /v1/tenant/* path`);
  }
  if (!slug) {
    throw new Error("tenantRequest: organization slug is required");
  }

  return withStepUpRetry(async () => {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    let url = `${base}${pathname}`;

    if (options?.query) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(options.query)) {
        if (value !== undefined && value !== "") {
          params.set(key, String(value));
        }
      }
      const qs = params.toString();
      if (qs) url += `?${qs}`;
    }

    const response = await fetch(url, {
      method,
      credentials: "include",
      signal: options?.signal,
      headers: {
        Accept: "application/json",
        [ORGANIZATION_HEADER]: slug,
        ...(options?.body !== undefined
          ? { "Content-Type": "application/json" }
          : {}),
      },
      body:
        options?.body !== undefined ? JSON.stringify(options.body) : undefined,
    });

    const payload = await response.json().catch(() => undefined);
    return unwrap<T>({ data: payload, response }, { silent: options?.silent });
  });
}

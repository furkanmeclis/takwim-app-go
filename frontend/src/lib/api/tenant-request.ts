import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api/client";
import { parseContentDispositionFilename } from "@/lib/api/content-disposition";
import {
  BRANCH_HEADER,
  getSelectedBranch,
  setSelectedBranch,
} from "@/lib/api/branch-selection";
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
  /**
   * Send the branch picked in the header (`X-Branch`). Default true; the
   * branch picker itself lists every accessible branch with false.
   */
  branch?: boolean;
};

/** API answer when `X-Branch` names a branch the member cannot use. */
const BRANCH_NOT_ALLOWED = "BRANCH_NOT_ALLOWED";

function errorCode(payload: unknown): string | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const error = (payload as { error?: { code?: unknown } }).error;
  return typeof error?.code === "string" ? error.code : undefined;
}

/**
 * BFF fetch for organization-scoped endpoints. `slug` is the organization of
 * the current `/t/{slug}` route; it is sent as `X-Organization` so every
 * request names its organization and tabs on different organizations stay
 * independent. The branch picked in the header goes along as `X-Branch`; a
 * stale pick (branch deleted or out of scope) is dropped and the request
 * retried for all branches. Only `/v1/tenant/*` paths are accepted: the BFF
 * forwards the headers on those paths only.
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

    const send = (branch: string | null) =>
      fetch(url, {
        method,
        credentials: "include",
        signal: options?.signal,
        headers: {
          Accept: "application/json",
          [ORGANIZATION_HEADER]: slug,
          ...(branch ? { [BRANCH_HEADER]: branch } : {}),
          ...(options?.body !== undefined
            ? { "Content-Type": "application/json" }
            : {}),
        },
        body:
          options?.body !== undefined
            ? JSON.stringify(options.body)
            : undefined,
      });

    const branch = options?.branch === false ? null : getSelectedBranch(slug);
    let response = await send(branch);
    let payload = await response.json().catch(() => undefined);
    if (
      branch &&
      response.status === 403 &&
      errorCode(payload) === BRANCH_NOT_ALLOWED
    ) {
      setSelectedBranch(slug, null);
      response = await send(null);
      payload = await response.json().catch(() => undefined);
    }
    return unwrap<T>({ data: payload, response }, { silent: options?.silent });
  });
}

function tenantUrl(
  path: string,
  query?: Record<string, string | number | undefined>,
) {
  const pathname = path.startsWith("/") ? path : `/${path}`;
  if (!pathname.startsWith(TENANT_PATH_PREFIX)) {
    throw new Error(`tenantRequest: ${pathname} is not a /v1/tenant/* path`);
  }
  let url = `${apiConfig.baseUrl.replace(/\/$/, "")}${pathname}`;
  if (query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== "") params.set(key, String(value));
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  return url;
}

/** Multipart upload to an organization-scoped endpoint (imports). */
export async function tenantFormRequest<T>(
  slug: string,
  method: string,
  path: string,
  formData: FormData,
): Promise<T> {
  if (!slug) throw new Error("tenantRequest: organization slug is required");
  const response = await fetch(tenantUrl(path), {
    method,
    credentials: "include",
    headers: { [ORGANIZATION_HEADER]: slug },
    body: formData,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response });
}

/** Binary download from an organization-scoped endpoint (export / sample). */
export async function tenantDownloadFile(
  slug: string,
  path: string,
  query?: Record<string, string | undefined>,
): Promise<{ blob: Blob; filename: string | null }> {
  if (!slug) throw new Error("tenantRequest: organization slug is required");
  const response = await fetch(tenantUrl(path, query), {
    credentials: "include",
    headers: { [ORGANIZATION_HEADER]: slug },
  });
  if (!response.ok) {
    throw new Error(`Download failed (${response.status})`);
  }
  const blob = await response.blob();
  const filename = parseContentDispositionFilename(
    response.headers.get("Content-Disposition"),
  );
  return { blob, filename };
}

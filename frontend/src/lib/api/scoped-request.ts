import {
  platformDownloadFile,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

/**
 * Which API shell a shared engine (io, bulk, search) talks to: the platform
 * (`/v1/platform/*`, `/v1/search`) or one organization (`/v1/tenant/*` with
 * `X-Organization`). Components get it from `useApiScope()`.
 */
export type ApiScope = { kind: "platform" } | { kind: "tenant"; slug: string };

export const PLATFORM_SCOPE: ApiScope = { kind: "platform" };

type ScopedOptions = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
};

/** JSON request in the given scope. */
export function scopedRequest<T>(
  scope: ApiScope,
  method: string,
  path: string,
  options?: ScopedOptions,
): Promise<T> {
  if (scope.kind === "tenant") {
    return tenantRequest<T>(scope.slug, method, path, options);
  }
  return platformRequest<T>(method, path, options);
}

/** Multipart request in the given scope. */
export function scopedFormRequest<T>(
  scope: ApiScope,
  method: string,
  path: string,
  formData: FormData,
): Promise<T> {
  if (scope.kind === "tenant") {
    return tenantFormRequest<T>(scope.slug, method, path, formData);
  }
  return platformFormRequest<T>(method, path, formData);
}

/** Binary download in the given scope. */
export function scopedDownloadFile(
  scope: ApiScope,
  path: string,
  query?: Record<string, string | undefined>,
) {
  if (scope.kind === "tenant") {
    return tenantDownloadFile(scope.slug, path, query);
  }
  return platformDownloadFile(path, query);
}

/** Engine job route prefix: `/v1/platform/exports` or `/v1/tenant/exports`. */
export function scopedJobPath(
  scope: ApiScope,
  engine: "exports" | "imports" | "bulk",
) {
  return scope.kind === "tenant"
    ? `/v1/tenant/${engine}`
    : `/v1/platform/${engine}`;
}

/** Stable react-query key segment for a scope. */
export function scopeKey(scope: ApiScope) {
  return scope.kind === "tenant" ? `tenant:${scope.slug}` : "platform";
}

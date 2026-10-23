"use client";

import { useMemo } from "react";

import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";
import { PLATFORM_SCOPE, type ApiScope } from "@/lib/api/scoped-request";

/**
 * API scope of the current shell: inside `/t/{slug}` shared engines (io,
 * bulk, search) call the organization's `/v1/tenant/*` routes; elsewhere the
 * platform ones.
 */
export function useApiScope(): ApiScope {
  const slug = useOptionalTenant()?.slug;
  return useMemo<ApiScope>(
    () => (slug ? { kind: "tenant", slug } : PLATFORM_SCOPE),
    [slug],
  );
}

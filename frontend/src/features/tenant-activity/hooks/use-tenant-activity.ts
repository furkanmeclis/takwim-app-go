"use client";

import { useQuery } from "@tanstack/react-query";

import { tenantActivityService } from "@/features/tenant-activity/services/tenant-activity.service";
import type { ListTenantActivityParams } from "@/features/tenant-activity/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";

/** Query keys carry the organization slug (see branches query keys). */
export const tenantActivityKeys = {
  all: (slug: string) => ["tenant", slug, "activity"] as const,
  list: (slug: string, params: ListTenantActivityParams) =>
    [...tenantActivityKeys.all(slug), "list", params] as const,
  facets: (slug: string) =>
    [...tenantActivityKeys.all(slug), "facets"] as const,
};

export function useTenantActivity(params: ListTenantActivityParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantActivityKeys.list(slug, params),
    queryFn: () => tenantActivityService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useTenantActivityFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantActivityKeys.facets(slug),
    queryFn: () => tenantActivityService.facets(slug),
    staleTime: 60_000,
  });
}

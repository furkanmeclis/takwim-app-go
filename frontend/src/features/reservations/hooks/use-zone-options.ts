"use client";

import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { useResourcesFacets } from "@/features/resources/hooks/use-resources";
import { usePermission } from "@/providers/permission-provider";

/**
 * Areas (resource zones) a party may ask for, from the resource facets of the
 * caller's branches. Empty when not `enabled`, when the member cannot read
 * resources, or when no resource has a zone; the form then asks for none.
 */
export function useZoneOptions(enabled: boolean) {
  const { can } = usePermission();
  const facets = useResourcesFacets({
    enabled: enabled && can(permissions.tenant.resources.read),
  });
  return useMemo(
    () =>
      (facets.data?.zone ?? [])
        .filter((facet) => facet.value)
        .map((facet) => ({ value: facet.value, label: facet.value })),
    [facets.data?.zone],
  );
}

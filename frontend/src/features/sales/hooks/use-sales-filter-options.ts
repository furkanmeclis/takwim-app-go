"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { staffKeys } from "@/features/staff/hooks/query-keys";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { staffService } from "@/features/staff/services/staff.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { usePermission } from "@/providers/permission-provider";

const STAFF_QUERY = { limit: 100, offset: 0, sort: "name" };

type Option = { value: string; label: string };

/**
 * Options of the branch and staff filters: the branches of the member's
 * scope and, when the member may read staff, the staff list.
 */
export function useSalesFilterOptions() {
  const { slug } = useTenant();
  const { can } = usePermission();
  const branches = useAccessibleBranches();
  const canReadStaff = can(permissions.tenant.staff.read);
  const staff = useQuery({
    queryKey: staffKeys.list(slug, STAFF_QUERY),
    queryFn: () => staffService.list(slug, STAFF_QUERY),
    enabled: canReadStaff,
    staleTime: 60_000,
  });

  const branchOptions = useMemo<Option[]>(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: branch.name,
      })),
    [branches.data],
  );
  const staffOptions = useMemo<Option[]>(
    () =>
      (staff.data?.items ?? []).map((member) => ({
        value: member.uuid,
        label: member.name,
      })),
    [staff.data],
  );

  return { branchOptions, staffOptions };
}

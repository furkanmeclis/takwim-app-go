"use client";

import { useMutation, useQuery } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import type { BranchPage } from "@/features/branches/types";
import { reportsService } from "@/features/reports/services/reports.service";
import type { ReportFilter, ReportSection } from "@/features/reports/types";
import { staffService } from "@/features/staff/services/staff.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const reportsKeys = {
  all: (slug: string) => ["tenant", slug, "reports"] as const,
  summary: (slug: string, filter: ReportFilter) =>
    [...reportsKeys.all(slug), "summary", filter] as const,
  branches: (slug: string) =>
    [...reportsKeys.all(slug), "branch-options"] as const,
  staff: (slug: string) => [...reportsKeys.all(slug), "staff-options"] as const,
};

export function useReport(filter: ReportFilter) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reportsKeys.summary(slug, filter),
    queryFn: () => reportsService.summary(slug, filter),
    placeholderData: (previous) => previous,
  });
}

export type FilterOption = { value: string; label: string };

/**
 * Branch and staff choices of the filter bar. Each list needs its own read
 * permission; without it the filter is not offered.
 */
export function useReportFilterOptions() {
  const { slug } = useTenant();
  const { can } = usePermission();
  const canBranches = can(permissions.tenant.branches.read);
  const canStaff = can(permissions.tenant.staff.read);
  const branches = useQuery({
    queryKey: reportsKeys.branches(slug),
    queryFn: () =>
      tenantRequest<BranchPage>(slug, "GET", "/v1/tenant/branches", {
        query: { limit: 100, sort: "name" },
        silent: true,
      }),
    enabled: canBranches && Boolean(slug),
    staleTime: 60_000,
  });
  const staff = useQuery({
    queryKey: reportsKeys.staff(slug),
    queryFn: () =>
      staffService.list(slug, { limit: 100, offset: 0, sort: "name" }),
    enabled: canStaff && Boolean(slug),
    staleTime: 60_000,
  });
  return {
    branches: canBranches
      ? (branches.data?.items ?? []).map<FilterOption>((b) => ({
          value: b.uuid,
          label: b.name,
        }))
      : null,
    staff:
      canStaff && !staff.isError
        ? (staff.data?.items ?? []).map<FilterOption>((s) => ({
            value: s.uuid,
            label: s.name,
          }))
        : null,
  };
}

/** Downloads the CSV of a section and tells the user when it fails. */
export function useReportExport() {
  const { slug } = useTenant();
  const { t, locale } = useLocale();
  return useMutation({
    mutationFn: async ({
      filter,
      section,
    }: {
      filter: ReportFilter;
      section: ReportSection;
    }) => {
      const { blob, filename } = await reportsService.exportCsv(
        slug,
        filter,
        section,
        locale,
      );
      triggerBrowserDownload(
        blob,
        filename ?? `rapor_${section}_${filter.date_from}.csv`,
      );
    },
    onSuccess: () => appToast.success(t("reports.export.done")),
    onError: () => appToast.error(t("reports.export.failed")),
  });
}

"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { useCashAccountOptions } from "@/features/cash/hooks/use-cash";
import { useFinanceCategories } from "@/features/finance/hooks/use-finance";
import { AUTO, NONE } from "@/features/finance/schemas/finance-forms";
import type { FinanceKind, FinanceRef } from "@/features/finance/types";
import { staffKeys } from "@/features/staff/hooks/query-keys";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { staffService } from "@/features/staff/services/staff.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const CATEGORY_QUERY = { limit: 200, offset: 0, sort: "sort_order" };
const STAFF_QUERY = {
  limit: 100,
  offset: 0,
  sort: "name",
  is_active: "true" as const,
};

type Option = { value: string; label: string };

/**
 * Pickers of the record forms: active categories of the kind (plus the one
 * a record keeps), active accounts of the member's scope, branches and
 * staff. Staff come from the staff list when the member may read it.
 */
export function useFinanceFormOptions({
  kind,
  enabled,
  keep,
}: {
  kind: FinanceKind;
  enabled: boolean;
  keep?: { category?: FinanceRef; staff?: FinanceRef | null };
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const categories = useFinanceCategories(CATEGORY_QUERY, enabled);
  const accounts = useCashAccountOptions(enabled);
  const branches = useAccessibleBranches();
  const canReadStaff = can(permissions.tenant.staff.read);
  const staff = useQuery({
    queryKey: staffKeys.list(slug, STAFF_QUERY),
    queryFn: () => staffService.list(slug, STAFF_QUERY),
    enabled: enabled && canReadStaff,
    staleTime: 60_000,
  });

  const categoryOptions = useMemo<Option[]>(() => {
    const items = (categories.data?.items ?? [])
      .filter((c) => c.kind === kind && c.status === "active")
      .map((c) => ({ value: c.uuid, label: c.name }));
    const kept = keep?.category;
    if (kept && !items.some((item) => item.value === kept.uuid)) {
      items.unshift({ value: kept.uuid, label: kept.name });
    }
    return items;
  }, [categories.data?.items, keep?.category, kind]);

  const accountOptions = useMemo<Option[]>(
    () =>
      (accounts.data?.items ?? [])
        .filter((account) => account.status === "active")
        .map((account) => ({
          value: account.uuid,
          label: account.branch_name
            ? `${account.name} · ${account.branch_name}`
            : `${account.name} · ${t("finance.head_office")}`,
        })),
    [accounts.data?.items, t],
  );

  const branchOptions = useMemo<Option[]>(
    () => [
      { value: AUTO, label: t("finance.fields.branch_auto") },
      ...(branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      })),
    ],
    [branches.data?.items, t],
  );

  const staffOptions = useMemo<Option[]>(() => {
    const items = canReadStaff
      ? (staff.data?.items ?? []).map((member) => ({
          value: member.uuid,
          label: member.name,
        }))
      : [];
    const kept = keep?.staff;
    if (kept && !items.some((item) => item.value === kept.uuid)) {
      items.unshift({ value: kept.uuid, label: kept.name });
    }
    return [{ value: NONE, label: t("finance.fields.staff_none") }, ...items];
  }, [canReadStaff, keep?.staff, staff.data?.items, t]);

  return {
    categoryOptions,
    accountOptions,
    branchOptions,
    staffOptions,
    accountsLoaded: accounts.isSuccess,
  };
}

"use client";

import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronsUpDown, Store } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { permissions } from "@/config/permissions";
import type { BranchPage } from "@/features/branches/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Every branch the member may access, unaffected by the current pick. */
function useBranchOptions(slug: string, enabled: boolean) {
  return useQuery({
    queryKey: ["tenant", slug, "branch-picker"],
    queryFn: () =>
      tenantRequest<BranchPage>(slug, "GET", "/v1/tenant/branches", {
        query: { limit: 100, sort: "name" },
        branch: false,
        silent: true,
      }),
    enabled: enabled && Boolean(slug),
    staleTime: 60_000,
  });
}

/**
 * Header branch picker of the tenant shell: one branch or "all branches".
 * The pick is sent with every request (`X-Branch`), so every list, count
 * and the overview follow it; switching refetches the organization's data.
 * Hidden for members with a single branch.
 */
export function BranchPicker() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const [selected, select] = useSelectedBranch(slug);
  const allowed = can(permissions.tenant.branches.read);
  const query = useBranchOptions(slug, allowed);
  const branches = query.data?.items ?? [];
  const current = branches.find((b) => b.uuid === selected) ?? null;

  // Any change of the pick (here, or a stale pick dropped by a request)
  // refetches the organization's data for the new view.
  const previous = useRef(selected);
  useEffect(() => {
    if (previous.current === selected) return;
    previous.current = selected;
    void queryClient.invalidateQueries({ queryKey: ["tenant", slug] });
    void queryClient.invalidateQueries({ queryKey: ["search"] });
  }, [queryClient, selected, slug]);

  // A pick that is no longer in the list (deleted, scope changed) is dropped.
  useEffect(() => {
    if (query.isSuccess && selected && !current) select(null);
  }, [current, query.isSuccess, select, selected]);

  if (!allowed || (branches.length < 2 && !current)) return null;

  const label = current?.name ?? t("tenant.branch_picker.all");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          className="h-9 max-w-56 min-w-0 gap-2 px-2"
          aria-label={t("tenant.branch_picker.label")}
        >
          <Store className="text-muted-foreground size-4 shrink-0" />
          <span className="hidden truncate text-sm sm:inline">{label}</span>
          <ChevronsUpDown className="text-muted-foreground size-3.5 shrink-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
          {t("tenant.branch_picker.title")}
        </DropdownMenuLabel>
        <DropdownMenuGroup>
          <DropdownMenuItem
            onSelect={() => select(null)}
            className="flex items-center gap-2"
          >
            <span className="flex-1">{t("tenant.branch_picker.all")}</span>
            {selected === null ? <Check className="size-4" /> : null}
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup className="max-h-80 overflow-y-auto">
          {branches.map((branch) => (
            <DropdownMenuItem
              key={branch.uuid}
              onSelect={() => select(branch.uuid)}
              className="flex items-center gap-2"
            >
              <span className="min-w-0 flex-1 truncate">{branch.name}</span>
              {branch.is_active ? null : (
                <Badge variant="secondary">
                  {t("tenant.branch_picker.inactive")}
                </Badge>
              )}
              {branch.uuid === selected ? <Check className="size-4" /> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

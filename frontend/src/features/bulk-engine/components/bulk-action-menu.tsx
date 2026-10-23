"use client";

import { ChevronDown, Layers } from "lucide-react";
import { useMemo } from "react";

import type {
  BulkActionDef,
  BulkResource,
  BulkSummary,
  SelectionScope,
} from "@/features/bulk-engine/types";
import { buildBulkTarget } from "@/features/bulk-engine/hooks/use-bulk-selection";
import { useBulkMutation } from "@/features/bulk-engine/hooks/use-bulk-mutation";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { usePermission } from "@/providers/permission-provider";
import { cn } from "@/lib/utils";

type BulkActionMenuProps = {
  resource: BulkResource;
  actions: BulkActionDef[];
  scope: SelectionScope;
  selectedCount: number;
  onComplete?: () => void;
  /** Summary of a run that finished inline (skipped / failed items). */
  onResult?: (action: BulkActionDef, summary: BulkSummary) => void;
  /**
   * Collects the params of a parameterised action (for example a dialog
   * asking the new status). Resolve `null` to cancel, `undefined` for an
   * action without params.
   */
  resolveParams?: (
    action: BulkActionDef,
  ) => Promise<Record<string, string> | null | undefined>;
};

export function BulkActionMenu({
  resource,
  actions,
  scope,
  selectedCount,
  onComplete,
  onResult,
  resolveParams,
}: BulkActionMenuProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { confirm, confirmDelete } = useDialogs();
  const bulkMutation = useBulkMutation();

  const allowed = useMemo(
    () => actions.filter((action) => can(action.permission)),
    [actions, can],
  );

  if (selectedCount <= 0 || allowed.length === 0) return null;

  const confirmAction = async (action: BulkActionDef) => {
    const description = action.confirm_key
      ? t(action.confirm_key, { count: selectedCount })
      : t("bulk.confirm_generic", { count: selectedCount });
    const confirmed = action.destructive
      ? await confirmDelete({ title: t(action.label_key), description })
      : await confirm({ title: t(action.label_key), description });
    return confirmed;
  };

  const runAction = async (action: BulkActionDef) => {
    const params = resolveParams ? await resolveParams(action) : undefined;
    if (params === null) return;
    if (action.confirm_key || action.destructive) {
      const ok = await confirmAction(action);
      if (!ok) return;
    }
    const target = params
      ? { ...buildBulkTarget(scope), params }
      : buildBulkTarget(scope);
    await bulkMutation.mutateAsync({
      resource,
      action: action.id,
      target,
      onComplete,
      onResult: onResult ? (summary) => onResult(action, summary) : undefined,
    });
  };

  if (allowed.length === 1) {
    const action = allowed[0]!;
    const Icon = action.icon;
    return (
      <Button
        type="button"
        size="sm"
        variant={action.destructive ? "destructive" : "secondary"}
        disabled={bulkMutation.isPending}
        className="h-8 gap-1.5"
        onClick={() => void runAction(action)}
      >
        <Icon className="size-3.5" />
        {t(action.label_key)}
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="secondary"
          disabled={bulkMutation.isPending}
          className="h-8 gap-1.5"
        >
          <Layers className="size-3.5" />
          {t("table.bulk")}
          <ChevronDown className="size-3.5 opacity-70" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        {allowed.map((action) => {
          const Icon = action.icon;
          return (
            <DropdownMenuItem
              key={action.id}
              className={cn(
                action.destructive && "text-destructive focus:text-destructive",
              )}
              onClick={() => void runAction(action)}
            >
              <Icon className="size-4" />
              {t(action.label_key)}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

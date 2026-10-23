"use client";

import { Eye, Pencil, PencilLine, Power, PowerOff, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Branch } from "@/features/branches/types";
import { useLocale } from "@/providers/locale-provider";

export type BranchRowActionHandlers = {
  onView: (branch: Branch) => void;
  onQuickEdit: (branch: Branch) => void;
  onEdit: (branch: Branch) => void;
  onToggleActive: (branch: Branch) => void;
  onDelete: (branch: Branch) => void;
};

export function BranchRowActions({
  branch,
  handlers,
}: {
  branch: Branch;
  handlers: BranchRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.branches.write;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("branches.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(branch),
    },
    {
      id: "quick-edit",
      label: t("branches.actions.quick_edit"),
      icon: PencilLine,
      permission: write,
      onSelect: () => handlers.onQuickEdit(branch),
    },
    {
      id: "edit",
      label: t("branches.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(branch),
    },
    {
      id: "toggle-active",
      label: branch.is_active
        ? t("branches.actions.deactivate")
        : t("branches.actions.activate"),
      icon: branch.is_active ? PowerOff : Power,
      permission: write,
      onSelect: () => handlers.onToggleActive(branch),
    },
    {
      id: "delete",
      label: t("branches.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(branch),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

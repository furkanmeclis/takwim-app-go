"use client";

import { Eye, Pencil, PencilLine, Power, PowerOff, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Staff } from "@/features/staff/types";
import { useLocale } from "@/providers/locale-provider";

export type StaffRowActionHandlers = {
  onView: (staff: Staff) => void;
  onQuickEdit: (staff: Staff) => void;
  onEdit: (staff: Staff) => void;
  onToggleActive: (staff: Staff) => void;
  onDelete: (staff: Staff) => void;
};

export function StaffRowActions({
  staff,
  handlers,
}: {
  staff: Staff;
  handlers: StaffRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.staff.write;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("staff.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(staff),
    },
    {
      id: "quick-edit",
      label: t("staff.actions.quick_edit"),
      icon: PencilLine,
      permission: write,
      onSelect: () => handlers.onQuickEdit(staff),
    },
    {
      id: "edit",
      label: t("staff.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(staff),
    },
    {
      id: "toggle-active",
      label: staff.is_active
        ? t("staff.actions.deactivate")
        : t("staff.actions.activate"),
      icon: staff.is_active ? PowerOff : Power,
      permission: write,
      onSelect: () => handlers.onToggleActive(staff),
    },
    {
      id: "delete",
      label: t("staff.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(staff),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

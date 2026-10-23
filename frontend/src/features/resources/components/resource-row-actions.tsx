"use client";

import { Eye, Pencil, PencilLine, Power, PowerOff, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Resource } from "@/features/resources/types";
import { useLocale } from "@/providers/locale-provider";

export type ResourceRowActionHandlers = {
  onView: (resource: Resource) => void;
  onQuickEdit: (resource: Resource) => void;
  onEdit: (resource: Resource) => void;
  onToggleActive: (resource: Resource) => void;
  onDelete: (resource: Resource) => void;
};

export function ResourceRowActions({
  resource,
  handlers,
}: {
  resource: Resource;
  handlers: ResourceRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.resources.write;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("resources.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(resource),
    },
    {
      id: "quick-edit",
      label: t("resources.actions.quick_edit"),
      icon: PencilLine,
      permission: write,
      onSelect: () => handlers.onQuickEdit(resource),
    },
    {
      id: "edit",
      label: t("resources.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(resource),
    },
    {
      id: "toggle-active",
      label: resource.is_active
        ? t("resources.actions.deactivate")
        : t("resources.actions.activate"),
      icon: resource.is_active ? PowerOff : Power,
      permission: write,
      onSelect: () => handlers.onToggleActive(resource),
    },
    {
      id: "delete",
      label: t("resources.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(resource),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

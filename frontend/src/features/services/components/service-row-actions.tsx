"use client";

import { Eye, Pencil, PencilLine, Power, PowerOff, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Service } from "@/features/services/types";
import { useLocale } from "@/providers/locale-provider";

export type ServiceRowActionHandlers = {
  onView: (service: Service) => void;
  onQuickEdit: (service: Service) => void;
  onEdit: (service: Service) => void;
  onToggleActive: (service: Service) => void;
  onDelete: (service: Service) => void;
};

export function ServiceRowActions({
  service,
  handlers,
}: {
  service: Service;
  handlers: ServiceRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.services.write;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("services.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(service),
    },
    {
      id: "quick-edit",
      label: t("services.actions.quick_edit"),
      icon: PencilLine,
      permission: write,
      onSelect: () => handlers.onQuickEdit(service),
    },
    {
      id: "edit",
      label: t("services.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(service),
    },
    {
      id: "toggle-active",
      label: service.is_active
        ? t("services.actions.deactivate")
        : t("services.actions.activate"),
      icon: service.is_active ? PowerOff : Power,
      permission: write,
      onSelect: () => handlers.onToggleActive(service),
    },
    {
      id: "delete",
      label: t("services.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(service),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

"use client";

import { Eye, Pencil, Power, PowerOff, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { ServiceBundle } from "@/features/service-bundles/types";
import { useLocale } from "@/providers/locale-provider";

export type BundleRowActionHandlers = {
  onView: (bundle: ServiceBundle) => void;
  onEdit: (bundle: ServiceBundle) => void;
  onToggleActive: (bundle: ServiceBundle) => void;
  onDelete: (bundle: ServiceBundle) => void;
};

export function BundleRowActions({
  bundle,
  handlers,
}: {
  bundle: ServiceBundle;
  handlers: BundleRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.services.write;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("service_bundles.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(bundle),
    },
    {
      id: "edit",
      label: t("service_bundles.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(bundle),
    },
    {
      id: "toggle-active",
      label: bundle.is_active
        ? t("service_bundles.actions.deactivate")
        : t("service_bundles.actions.activate"),
      icon: bundle.is_active ? PowerOff : Power,
      permission: write,
      onSelect: () => handlers.onToggleActive(bundle),
    },
    {
      id: "delete",
      label: t("service_bundles.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(bundle),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

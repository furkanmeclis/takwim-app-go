"use client";

import {
  ArrowLeftRight,
  Eye,
  Pencil,
  PencilLine,
  Power,
  PowerOff,
  Trash2,
} from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Product } from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type ProductRowActionHandlers = {
  onView: (product: Product) => void;
  onQuickEdit: (product: Product) => void;
  onEdit: (product: Product) => void;
  onMovement: (product: Product) => void;
  onToggleActive: (product: Product) => void;
  onDelete: (product: Product) => void;
};

export function ProductRowActions({
  product,
  handlers,
}: {
  product: Product;
  handlers: ProductRowActionHandlers;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const write = permissions.tenant.products.write;
  const canMove =
    can(permissions.tenant.products.stock) ||
    can(permissions.tenant.products.sell);
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("products.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(product),
    },
    {
      id: "quick-edit",
      label: t("products.actions.quick_edit"),
      icon: PencilLine,
      permission: write,
      onSelect: () => handlers.onQuickEdit(product),
    },
    {
      id: "edit",
      label: t("products.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(product),
    },
    ...(canMove
      ? [
          {
            id: "movement",
            label: t("products.actions.record_movement"),
            icon: ArrowLeftRight,
            onSelect: () => handlers.onMovement(product),
          } satisfies EntityRowAction,
        ]
      : []),
    {
      id: "toggle-active",
      label: product.is_active
        ? t("products.actions.deactivate")
        : t("products.actions.activate"),
      icon: product.is_active ? PowerOff : Power,
      permission: write,
      onSelect: () => handlers.onToggleActive(product),
    },
    {
      id: "delete",
      label: t("products.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(product),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

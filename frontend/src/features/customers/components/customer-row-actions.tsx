"use client";

import { Eye, Pencil, PencilLine, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Customer } from "@/features/customers/types";
import { useLocale } from "@/providers/locale-provider";

export type CustomerRowActionHandlers = {
  onView: (customer: Customer) => void;
  onQuickEdit: (customer: Customer) => void;
  onEdit: (customer: Customer) => void;
  onDelete: (customer: Customer) => void;
};

export function CustomerRowActions({
  customer,
  handlers,
}: {
  customer: Customer;
  handlers: CustomerRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.customers.write;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("customers.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(customer),
    },
    {
      id: "quick-edit",
      label: t("customers.actions.quick_edit"),
      icon: PencilLine,
      permission: write,
      onSelect: () => handlers.onQuickEdit(customer),
    },
    {
      id: "edit",
      label: t("customers.actions.edit"),
      icon: Pencil,
      permission: write,
      onSelect: () => handlers.onEdit(customer),
    },
    {
      id: "delete",
      label: t("customers.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(customer),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

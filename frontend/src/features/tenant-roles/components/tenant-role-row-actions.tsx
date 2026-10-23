"use client";

import { Eye, Pencil, Trash2 } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { TenantRole } from "@/features/tenant-roles/types";
import { useLocale } from "@/providers/locale-provider";

export type TenantRoleRowActionHandlers = {
  onOpen: (role: TenantRole) => void;
  onDelete: (role: TenantRole) => void;
};

/** Template roles are read-only: only "view" is offered for them. */
export function TenantRoleRowActions({
  role,
  handlers,
}: {
  role: TenantRole;
  handlers: TenantRoleRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.roles.write;
  const actions: EntityRowAction[] = role.is_template
    ? [
        {
          id: "view",
          label: t("tenant_roles.actions.view"),
          icon: Eye,
          onSelect: () => handlers.onOpen(role),
        },
      ]
    : [
        {
          id: "edit",
          label: t("tenant_roles.actions.edit"),
          icon: Pencil,
          permission: write,
          onSelect: () => handlers.onOpen(role),
        },
        {
          id: "delete",
          label: t("tenant_roles.actions.delete"),
          icon: Trash2,
          permission: write,
          variant: "destructive",
          disabled: role.member_count > 0,
          onSelect: () => handlers.onDelete(role),
        },
      ];
  return <EntityRowActions actions={actions} />;
}

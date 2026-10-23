"use client";

import { KeyRound, UserMinus } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { TenantMember } from "@/features/tenant-members/types";
import { useLocale } from "@/providers/locale-provider";

export type MemberRowActionHandlers = {
  onEditAccess: (member: TenantMember) => void;
  onRemove: (member: TenantMember) => void;
};

export function MemberRowActions({
  member,
  handlers,
}: {
  member: TenantMember;
  handlers: MemberRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.members.write;
  const actions: EntityRowAction[] = [
    {
      id: "edit-access",
      label: t("tenant_members.actions.edit_access"),
      icon: KeyRound,
      permission: write,
      onSelect: () => handlers.onEditAccess(member),
    },
    {
      id: "remove",
      label: t("tenant_members.actions.remove"),
      icon: UserMinus,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onRemove(member),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

"use client";

import {
  Archive,
  ArchiveRestore,
  ArrowDownUp,
  ArrowLeftRight,
  Eye,
  Lock,
  Pencil,
} from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { CashAccount } from "@/features/cash/types";
import { useLocale } from "@/providers/locale-provider";

export type CashAccountRowActionHandlers = {
  onView: (account: CashAccount) => void;
  onMovement: (account: CashAccount) => void;
  onTransfer: (account: CashAccount) => void;
  onClose: (account: CashAccount) => void;
  onEdit: (account: CashAccount) => void;
  onToggleArchive: (account: CashAccount) => void;
};

export function CashAccountRowActions({
  account,
  handlers,
}: {
  account: CashAccount;
  handlers: CashAccountRowActionHandlers;
}) {
  const { t } = useLocale();
  const cash = permissions.tenant.cash;
  const active = account.status === "active";
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("cash.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(account),
    },
  ];
  if (active) {
    actions.push(
      {
        id: "movement",
        label: t("cash.actions.movement"),
        icon: ArrowDownUp,
        permission: cash.write,
        onSelect: () => handlers.onMovement(account),
      },
      {
        id: "transfer",
        label: t("cash.actions.transfer"),
        icon: ArrowLeftRight,
        permission: cash.write,
        onSelect: () => handlers.onTransfer(account),
      },
    );
    if (account.kind === "cash") {
      actions.push({
        id: "close",
        label: t("cash.actions.close_day"),
        icon: Lock,
        permission: cash.close,
        onSelect: () => handlers.onClose(account),
      });
    }
  }
  actions.push(
    {
      id: "edit",
      label: t("cash.actions.edit"),
      icon: Pencil,
      permission: cash.manage,
      onSelect: () => handlers.onEdit(account),
    },
    {
      id: "toggle-archive",
      label: active ? t("cash.actions.archive") : t("cash.actions.activate"),
      icon: active ? Archive : ArchiveRestore,
      permission: cash.manage,
      onSelect: () => handlers.onToggleArchive(account),
    },
  );
  return <EntityRowActions actions={actions} />;
}

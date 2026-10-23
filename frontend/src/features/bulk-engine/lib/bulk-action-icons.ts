import {
  BadgeMinus,
  BadgePlus,
  Ban,
  CalendarCheck,
  CalendarX,
  CircleCheck,
  CircleOff,
  Trash2,
  UserCheck,
  UserX,
  type LucideIcon,
} from "lucide-react";

import type {
  BulkActionDef,
  BulkActionMeta,
  BulkResource,
} from "@/features/bulk-engine/types";

const bulkActionIconCatalog: Record<
  BulkResource,
  Record<string, LucideIcon>
> = {
  "platform.users": {
    disable: UserX,
    enable: UserCheck,
  },
  "platform.roles": {
    delete: Trash2,
  },
  "platform.organizations": {
    activate: CircleCheck,
    suspend: CircleOff,
  },
  "tenant.branches": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.staff": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.resources": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.services": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.products": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.customers": {
    add_tag: BadgePlus,
    remove_tag: BadgeMinus,
    delete: Trash2,
  },
  "tenant.reservations": {
    accept: CalendarCheck,
    reject: CalendarX,
    cancel: Ban,
  },
};

export function resolveBulkActionIcon(
  resource: BulkResource,
  actionId: string,
): LucideIcon {
  const icon = bulkActionIconCatalog[resource]?.[actionId];
  if (!icon) {
    throw new Error(
      `bulk-engine: missing icon for ${resource} action "${actionId}"`,
    );
  }
  return icon;
}

/** Attach mandatory icons to meta bulk actions for UI rendering. */
export function resolveBulkActionsWithIcons(
  resource: BulkResource,
  actions: BulkActionMeta[],
): BulkActionDef[] {
  return actions.map((action) => ({
    ...action,
    icon: resolveBulkActionIcon(resource, action.id),
  }));
}

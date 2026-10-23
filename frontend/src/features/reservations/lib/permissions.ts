"use client";

import { permissions } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

/** Full read, or the member's own calendar (`.own`). */
export const READ_ANY = [
  permissions.tenant.reservations.read,
  permissions.tenant.reservations.own,
];

/** Full write, or the member's own calendar (`.own`). */
export const WRITE_ANY = [
  permissions.tenant.reservations.write,
  permissions.tenant.reservations.own,
];

export function useCanWriteReservations() {
  const { canAny } = usePermission();
  return canAny(WRITE_ANY);
}

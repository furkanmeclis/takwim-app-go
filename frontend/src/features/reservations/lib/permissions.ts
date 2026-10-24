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

/** Who sees the payment follow-up list (refund, collect or finance read). */
export const FOLLOW_UP_ANY = [
  permissions.tenant.payment.refund,
  permissions.tenant.payment.collect,
  permissions.tenant.finance.read,
];

export function useCanWriteReservations() {
  const { canAny } = usePermission();
  return canAny(WRITE_ANY);
}

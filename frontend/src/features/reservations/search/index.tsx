import { CalendarClock } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of reservations. Inside `/t/{slug}` the palette searches
 * `GET /v1/tenant/search`: number (with or without `#`), customer name and
 * phone match, inside the member's branch scope; a member with only its own
 * calendar finds the bookings it made or serves.
 */
export const reservationsSearchSpec = defineSearchSpec({
  id: "reservations",
  labelKey: "search.specs_reservations",
  icon: CalendarClock,
  anyPermission: [
    permissions.tenant.reservations.read,
    permissions.tenant.reservations.own,
  ],
});

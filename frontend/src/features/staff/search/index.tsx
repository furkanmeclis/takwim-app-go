import { Contact } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of staff. Inside `/t/{slug}` the palette searches
 * `GET /v1/tenant/search`, which keeps hits in the organization and the
 * member's branches.
 */
export const staffSearchSpec = defineSearchSpec({
  id: "staff",
  labelKey: "search.specs_staff",
  icon: Contact,
  permission: permissions.tenant.staff.read,
});

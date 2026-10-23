import { ConciergeBell } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of services. Inside `/t/{slug}` the palette searches
 * `GET /v1/tenant/search`, which keeps hits in the organization.
 */
export const servicesSearchSpec = defineSearchSpec({
  id: "services",
  labelKey: "search.specs_services",
  icon: ConciergeBell,
  permission: permissions.tenant.services.read,
});

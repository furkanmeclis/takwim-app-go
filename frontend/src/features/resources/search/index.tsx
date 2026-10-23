import { Armchair } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of resources. Inside `/t/{slug}` the palette searches
 * `GET /v1/tenant/search`, which keeps hits in the organization and the
 * member's branches.
 */
export const resourcesSearchSpec = defineSearchSpec({
  id: "resources",
  labelKey: "search.specs_resources",
  icon: Armchair,
  permission: permissions.tenant.resources.read,
});

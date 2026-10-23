import { Store } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of branches. Inside `/t/{slug}` the palette lists it from
 * `GET /v1/tenant/search/specs` and searches `GET /v1/tenant/search`, which
 * filters hits to the current organization.
 */
export const branchesSearchSpec = defineSearchSpec({
  id: "branches",
  labelKey: "search.specs_branches",
  icon: Store,
  permission: permissions.tenant.branches.read,
});

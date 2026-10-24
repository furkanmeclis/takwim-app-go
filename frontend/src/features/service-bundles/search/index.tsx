import { Boxes } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of combined service bundles (TEC-985). Inside `/t/{slug}` the
 * palette searches `GET /v1/tenant/search`, which keeps hits in the
 * organization; a hit opens the bundle's detail page.
 */
export const serviceBundlesSearchSpec = defineSearchSpec({
  id: "service_bundles",
  labelKey: "search.specs_service_bundles",
  icon: Boxes,
  permission: permissions.tenant.services.read,
});

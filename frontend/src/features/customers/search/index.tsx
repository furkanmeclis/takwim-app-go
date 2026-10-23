import { UserRound } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of customers. Inside `/t/{slug}` the palette searches
 * `GET /v1/tenant/search`, which keeps hits in the organization; name,
 * email and phone (with or without the leading 0) match.
 */
export const customersSearchSpec = defineSearchSpec({
  id: "customers",
  labelKey: "search.specs_customers",
  icon: UserRound,
  permission: permissions.tenant.customers.read,
});

import { Wallet } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of cash and bank accounts. Inside `/t/{slug}` the palette
 * searches `GET /v1/tenant/search`, which keeps hits in the organization and
 * the member's branches (business-wide accounts only for all-branch members).
 */
export const cashAccountsSearchSpec = defineSearchSpec({
  id: "cash_accounts",
  labelKey: "search.specs_cash_accounts",
  icon: Wallet,
  permission: permissions.tenant.cash.read,
});

import { LayoutDashboard } from "lucide-react";

import { branchesSearchSpec } from "@/features/branches/search";
import {
  defineSearchCatalog,
  defineSearchSpec,
} from "@/features/search-engine/define";

/**
 * Local command-palette specs. Remote record specs come from
 * GET /v1/search/specs (platform) or GET /v1/tenant/search/specs (organization).
 */
export const searchCatalog = defineSearchCatalog({
  id: "app",
  specs: [
    defineSearchSpec({
      id: "pages",
      labelKey: "search.specs_pages",
      icon: LayoutDashboard,
      local: true,
    }),
    branchesSearchSpec,
  ],
});

import { LayoutDashboard } from "lucide-react";

import { branchesSearchSpec } from "@/features/branches/search";
import { cashAccountsSearchSpec } from "@/features/cash/search";
import { customersSearchSpec } from "@/features/customers/search";
import { reservationsSearchSpec } from "@/features/reservations/search";
import { resourcesSearchSpec } from "@/features/resources/search";
import { productsSearchSpec } from "@/features/products/search";
import { serviceBundlesSearchSpec } from "@/features/service-bundles/search";
import { servicesSearchSpec } from "@/features/services/search";
import { staffSearchSpec } from "@/features/staff/search";
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
    staffSearchSpec,
    resourcesSearchSpec,
    servicesSearchSpec,
    serviceBundlesSearchSpec,
    productsSearchSpec,
    customersSearchSpec,
    reservationsSearchSpec,
    cashAccountsSearchSpec,
  ],
});

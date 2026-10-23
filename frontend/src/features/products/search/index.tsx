import { Package } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of products (name, stock code, barcode). Inside `/t/{slug}`
 * the palette searches `GET /v1/tenant/search`, which keeps hits in the
 * organization.
 */
export const productsSearchSpec = defineSearchSpec({
  id: "products",
  labelKey: "search.specs_products",
  icon: Package,
  permission: permissions.tenant.products.read,
});

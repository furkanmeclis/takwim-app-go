import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Product = Schemas["TenantProduct"];
export type ProductPage = Schemas["TenantProductPage"];
export type ProductFacets = Schemas["TenantProductFacets"];
export type ProductOffer = Schemas["TenantProductOffer"];
export type ProductVariant = Schemas["TenantProductVariant"];
export type ProductStock = Schemas["TenantProductStock"];
export type ProductStockLevel = Schemas["TenantProductStockLevel"];
export type ProductLookupMatch = Schemas["TenantProductLookupMatch"];
export type ProductCreateRequest = Schemas["TenantProductCreateRequest"];
export type ProductPatchRequest = Schemas["TenantProductPatchRequest"];
export type ProductVariantCreateRequest =
  Schemas["TenantProductVariantCreateRequest"];
export type ProductVariantPatchRequest =
  Schemas["TenantProductVariantPatchRequest"];
export type ProductOffersRequest = Schemas["TenantProductOffersRequest"];
export type ProductActivityPage = Schemas["BranchActivityPage"];

export type StockMovement = Schemas["TenantStockMovement"];
export type StockMovementPage = Schemas["TenantStockMovementPage"];
export type StockMovementFacets = Schemas["TenantStockMovementFacets"];
export type StockMovementRequest = Schemas["TenantStockMovementRequest"];
export type StockMovementResult = Schemas["TenantStockMovementResult"];

export type ProductUnit = Product["unit"];
export type StockStatus = ProductStock["status"];
/** Ledger row kinds (a transfer writes `transfer_out` + `transfer_in`). */
export type MovementKind = StockMovement["kind"];
/** Kinds a member records (`transfer` becomes two ledger rows). */
export type MovementRequestKind = StockMovementRequest["kind"];

/** List query of `GET /v1/tenant/products`. */
export type ListProductsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated branch uuids (products sold there). */
  branch?: string;
  /** Comma-separated `in` / `low` / `out`. */
  stock_status?: string;
  is_active?: "true" | "false";
  show_online?: "true" | "false";
};

/** List query of `GET /v1/tenant/stock-movements`. */
export type ListStockMovementsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  kind?: string;
  branch?: string;
  product?: string;
  staff?: string;
  from?: string;
  to?: string;
};

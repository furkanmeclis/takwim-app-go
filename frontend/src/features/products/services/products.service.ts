import type { ResourceMeta } from "@/features/io/types";
import type {
  ListProductsParams,
  ListStockMovementsParams,
  Product,
  ProductActivityPage,
  ProductCreateRequest,
  ProductFacets,
  ProductLookupMatch,
  ProductOffersRequest,
  ProductPage,
  ProductPatchRequest,
  ProductVariantCreateRequest,
  ProductVariantPatchRequest,
  StockMovement,
  StockMovementFacets,
  StockMovementPage,
  StockMovementRequest,
  StockMovementResult,
} from "@/features/products/types";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/products";
const MOVEMENTS = "/v1/tenant/stock-movements";

/**
 * Product catalog and stock ledger of one organization. Every call names
 * the organization through `tenantRequest` (`X-Organization: {slug}`);
 * offers, stock levels and movements are limited to the member's branches
 * by the API.
 */
export const productsService = {
  list(slug: string, params: ListProductsParams) {
    return tenantRequest<ProductPage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<ProductFacets>(slug, "GET", `${BASE}/facets`);
  },

  /** Exact stock code or barcode match (product or variant). */
  lookup(slug: string, code: string) {
    return tenantRequest<{ items: ProductLookupMatch[] }>(
      slug,
      "GET",
      `${BASE}/lookup`,
      { query: { code } },
    );
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Product>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: ProductCreateRequest) {
    return tenantRequest<Product>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: ProductPatchRequest) {
    return tenantRequest<Product>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  setOffers(slug: string, uuid: string, body: ProductOffersRequest) {
    return tenantRequest<Product>(slug, "PUT", `${BASE}/${uuid}/offers`, {
      body,
    });
  },

  createVariant(slug: string, uuid: string, body: ProductVariantCreateRequest) {
    return tenantRequest<Product>(slug, "POST", `${BASE}/${uuid}/variants`, {
      body,
    });
  },

  updateVariant(
    slug: string,
    uuid: string,
    variantUuid: string,
    body: ProductVariantPatchRequest,
  ) {
    return tenantRequest<Product>(
      slug,
      "PATCH",
      `${BASE}/${uuid}/variants/${variantUuid}`,
      { body },
    );
  },

  removeVariant(slug: string, uuid: string, variantUuid: string) {
    return tenantRequest<Product>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/variants/${variantUuid}`,
    );
  },

  uploadImage(slug: string, uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<Product>(
      slug,
      "PUT",
      `${BASE}/${uuid}/image`,
      form,
    );
  },

  removeImage(slug: string, uuid: string) {
    return tenantRequest<Product>(slug, "DELETE", `${BASE}/${uuid}/image`);
  },

  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<ProductActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },

  movements(slug: string, params: ListStockMovementsParams) {
    return tenantRequest<StockMovementPage>(slug, "GET", MOVEMENTS, {
      query: params,
    });
  },

  movementsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${MOVEMENTS}/meta`);
  },

  movementsFacets(slug: string) {
    return tenantRequest<StockMovementFacets>(
      slug,
      "GET",
      `${MOVEMENTS}/facets`,
    );
  },

  movement(slug: string, uuid: string) {
    return tenantRequest<StockMovement>(slug, "GET", `${MOVEMENTS}/${uuid}`);
  },

  recordMovement(slug: string, body: StockMovementRequest) {
    return tenantRequest<StockMovementResult>(slug, "POST", MOVEMENTS, {
      body,
    });
  },
};

import type {
  MovementKind,
  MovementRequestKind,
  ProductUnit,
  StockStatus,
} from "@/features/products/types";

/** Resource slug shared by io / bulk / activity / live lists. */
export const PRODUCTS_RESOURCE = "tenant.products" as const;

export const PRODUCT_UNITS: ProductUnit[] = [
  "piece",
  "bottle",
  "box",
  "pack",
  "tube",
  "set",
];
export const STOCK_STATUSES: StockStatus[] = ["in", "low", "out"];
export const MOVEMENT_KINDS: MovementKind[] = [
  "receipt",
  "sale",
  "return",
  "adjustment",
  "transfer_out",
  "transfer_in",
];
export const MOVEMENT_REQUEST_KINDS: MovementRequestKind[] = [
  "receipt",
  "sale",
  "return",
  "adjustment",
  "transfer",
];

/** API limits (see `TenantProductCreateRequest` / `TenantStockMovementRequest`). */
export const MAX_LOW_STOCK = 1000000;
export const MAX_QUANTITY = 100000;
export const MAX_SORT_ORDER = 100000;
export const MAX_PRODUCT_IMAGE_BYTES = 5 * 1024 * 1024;

export const UNIT_LABEL_KEYS: Record<ProductUnit, string> = {
  piece: "products.unit.piece",
  bottle: "products.unit.bottle",
  box: "products.unit.box",
  pack: "products.unit.pack",
  tube: "products.unit.tube",
  set: "products.unit.set",
};

export const STOCK_STATUS_LABEL_KEYS: Record<StockStatus, string> = {
  in: "products.stock_status.in",
  low: "products.stock_status.low",
  out: "products.stock_status.out",
};

export const STOCK_STATUS_TONES: Record<
  StockStatus,
  "success" | "warning" | "danger"
> = {
  in: "success",
  low: "warning",
  out: "danger",
};

export const MOVEMENT_KIND_LABEL_KEYS: Record<MovementKind, string> = {
  receipt: "products.movements.kind.receipt",
  sale: "products.movements.kind.sale",
  return: "products.movements.kind.return",
  adjustment: "products.movements.kind.adjustment",
  transfer_out: "products.movements.kind.transfer_out",
  transfer_in: "products.movements.kind.transfer_in",
};

export const MOVEMENT_REQUEST_KIND_LABEL_KEYS: Record<
  MovementRequestKind,
  string
> = {
  receipt: "products.movements.kind.receipt",
  sale: "products.movements.kind.sale",
  return: "products.movements.kind.return",
  adjustment: "products.movements.kind.adjustment",
  transfer: "products.movements.kind.transfer",
};

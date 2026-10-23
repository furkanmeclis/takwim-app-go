"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { productsKeys } from "@/features/products/hooks/query-keys";
import { productsService } from "@/features/products/services/products.service";
import type {
  ListProductsParams,
  ListStockMovementsParams,
  ProductCreateRequest,
  ProductOffersRequest,
  ProductPatchRequest,
  ProductVariantCreateRequest,
  ProductVariantPatchRequest,
  StockMovementRequest,
} from "@/features/products/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useProductsList(params: ListProductsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.list(slug, params),
    queryFn: () => productsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useProductsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.meta(slug),
    queryFn: () => productsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useProductsFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.facets(slug),
    queryFn: () => productsService.facets(slug),
    staleTime: 60_000,
  });
}

export function useProduct(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.detail(slug, uuid ?? ""),
    queryFn: () => productsService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useProductActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...productsKeys.activity(slug, uuid), params],
    queryFn: () => productsService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useStockMovementsList(
  params: ListStockMovementsParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.movementList(slug, params),
    queryFn: () => productsService.movements(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useStockMovementsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.movementMeta(slug),
    queryFn: () => productsService.movementsMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useStockMovementsFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: productsKeys.movementFacets(slug),
    queryFn: () => productsService.movementsFacets(slug),
    staleTime: 60_000,
  });
}

function useInvalidateProducts() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: productsKeys.all(slug) });
}

/** One mutation with a success toast that refreshes every products query. */
function useProductsMutation<TVars, TData>(
  fn: (slug: string, vars: TVars) => Promise<TData>,
  toastKey: string,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateProducts();
  return useMutation({
    mutationFn: (vars: TVars) => fn(slug, vars),
    onSuccess: () => {
      appToast.success(t(toastKey));
      void invalidate();
    },
  });
}

export function useCreateProduct() {
  return useProductsMutation(
    (slug, body: ProductCreateRequest) => productsService.create(slug, body),
    "products.toast.created",
  );
}

export function useUpdateProduct() {
  return useProductsMutation(
    (slug, { uuid, body }: { uuid: string; body: ProductPatchRequest }) =>
      productsService.update(slug, uuid, body),
    "products.toast.updated",
  );
}

export function useDeleteProduct() {
  return useProductsMutation(
    (slug, uuid: string) => productsService.remove(slug, uuid),
    "products.toast.deleted",
  );
}

export function useSetProductOffers(uuid: string) {
  return useProductsMutation(
    (slug, body: ProductOffersRequest) =>
      productsService.setOffers(slug, uuid, body),
    "products.offers.toast.saved",
  );
}

export function useCreateProductVariant(uuid: string) {
  return useProductsMutation(
    (slug, body: ProductVariantCreateRequest) =>
      productsService.createVariant(slug, uuid, body),
    "products.variants.toast.created",
  );
}

export function useUpdateProductVariant(uuid: string) {
  return useProductsMutation(
    (
      slug,
      {
        variantUuid,
        body,
      }: { variantUuid: string; body: ProductVariantPatchRequest },
    ) => productsService.updateVariant(slug, uuid, variantUuid, body),
    "products.variants.toast.updated",
  );
}

export function useDeleteProductVariant(uuid: string) {
  return useProductsMutation(
    (slug, variantUuid: string) =>
      productsService.removeVariant(slug, uuid, variantUuid),
    "products.variants.toast.deleted",
  );
}

export function useUploadProductImage(uuid: string) {
  return useProductsMutation(
    (slug, file: File) => productsService.uploadImage(slug, uuid, file),
    "products.image.toast.uploaded",
  );
}

export function useRemoveProductImage(uuid: string) {
  return useProductsMutation(
    (slug, _: void) => productsService.removeImage(slug, uuid),
    "products.image.toast.removed",
  );
}

export function useRecordStockMovement() {
  return useProductsMutation(
    (slug, body: StockMovementRequest) =>
      productsService.recordMovement(slug, body),
    "products.movements.toast.recorded",
  );
}

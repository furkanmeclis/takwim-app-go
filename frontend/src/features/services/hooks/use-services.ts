"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { servicesKeys } from "@/features/services/hooks/query-keys";
import { servicesService } from "@/features/services/services/services.service";
import type {
  ListServicesParams,
  ServiceCategoryCreateRequest,
  ServiceCategoryPatchRequest,
  ServiceCreateRequest,
  ServiceOffersRequest,
  ServicePatchRequest,
  ServiceStaffRequest,
} from "@/features/services/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useServicesList(params: ListServicesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: servicesKeys.list(slug, params),
    queryFn: () => servicesService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useServicesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: servicesKeys.meta(slug),
    queryFn: () => servicesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useServicesFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: servicesKeys.facets(slug),
    queryFn: () => servicesService.facets(slug),
    staleTime: 60_000,
  });
}

export function useServiceCategories() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: servicesKeys.categories(slug),
    queryFn: () => servicesService.categories(slug),
    staleTime: 60_000,
  });
}

export function useService(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: servicesKeys.detail(slug, uuid ?? ""),
    queryFn: () => servicesService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useServiceActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...servicesKeys.activity(slug, uuid), params],
    queryFn: () => servicesService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateServices() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: servicesKeys.all(slug) });
}

/** One mutation with a success toast that refreshes every services query. */
function useServicesMutation<TVars, TData>(
  fn: (slug: string, vars: TVars) => Promise<TData>,
  toastKey: string,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateServices();
  return useMutation({
    mutationFn: (vars: TVars) => fn(slug, vars),
    onSuccess: () => {
      appToast.success(t(toastKey));
      void invalidate();
    },
  });
}

export function useCreateService() {
  return useServicesMutation(
    (slug, body: ServiceCreateRequest) => servicesService.create(slug, body),
    "services.toast.created",
  );
}

export function useUpdateService() {
  return useServicesMutation(
    (slug, { uuid, body }: { uuid: string; body: ServicePatchRequest }) =>
      servicesService.update(slug, uuid, body),
    "services.toast.updated",
  );
}

export function useDeleteService() {
  return useServicesMutation(
    (slug, uuid: string) => servicesService.remove(slug, uuid),
    "services.toast.deleted",
  );
}

export function useSetServiceOffers(uuid: string) {
  return useServicesMutation(
    (slug, body: ServiceOffersRequest) =>
      servicesService.setOffers(slug, uuid, body),
    "services.offers.toast.saved",
  );
}

export function useSetServiceStaff(uuid: string) {
  return useServicesMutation(
    (slug, body: ServiceStaffRequest) =>
      servicesService.setStaff(slug, uuid, body),
    "services.staff.toast.saved",
  );
}

export function useUploadServiceImage(uuid: string) {
  return useServicesMutation(
    (slug, file: File) => servicesService.uploadImage(slug, uuid, file),
    "services.image.toast.uploaded",
  );
}

export function useRemoveServiceImage(uuid: string) {
  return useServicesMutation(
    (slug, _: void) => servicesService.removeImage(slug, uuid),
    "services.image.toast.removed",
  );
}

export function useCreateServiceCategory() {
  return useServicesMutation(
    (slug, body: ServiceCategoryCreateRequest) =>
      servicesService.createCategory(slug, body),
    "services.categories.toast.created",
  );
}

export function useUpdateServiceCategory() {
  return useServicesMutation(
    (
      slug,
      { uuid, body }: { uuid: string; body: ServiceCategoryPatchRequest },
    ) => servicesService.updateCategory(slug, uuid, body),
    "services.categories.toast.updated",
  );
}

export function useDeleteServiceCategory() {
  return useServicesMutation(
    (slug, uuid: string) => servicesService.removeCategory(slug, uuid),
    "services.categories.toast.deleted",
  );
}

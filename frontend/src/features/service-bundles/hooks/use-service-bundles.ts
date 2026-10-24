"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { serviceBundlesKeys } from "@/features/service-bundles/hooks/query-keys";
import { serviceBundlesService } from "@/features/service-bundles/services/service-bundles.service";
import type {
  ListServiceBundlesParams,
  ServiceBundleCreateRequest,
  ServiceBundlePatchRequest,
} from "@/features/service-bundles/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useServiceBundlesList(
  params: ListServiceBundlesParams,
  options: { enabled?: boolean } = {},
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: serviceBundlesKeys.list(slug, params),
    queryFn: () => serviceBundlesService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: options.enabled ?? true,
  });
}

export function useServiceBundlesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: serviceBundlesKeys.meta(slug),
    queryFn: () => serviceBundlesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useServiceBundlesFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: serviceBundlesKeys.facets(slug),
    queryFn: () => serviceBundlesService.facets(slug),
    staleTime: 60_000,
  });
}

export function useServiceBundle(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: serviceBundlesKeys.detail(slug, uuid ?? ""),
    queryFn: () => serviceBundlesService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useServiceBundleActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...serviceBundlesKeys.activity(slug, uuid), params],
    queryFn: () => serviceBundlesService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

/** One mutation with a success toast that refreshes every bundle query. */
function useBundlesMutation<TVars, TData>(
  fn: (slug: string, vars: TVars) => Promise<TData>,
  toastKey: string,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: TVars) => fn(slug, vars),
    onSuccess: () => {
      appToast.success(t(toastKey));
      void queryClient.invalidateQueries({
        queryKey: serviceBundlesKeys.all(slug),
      });
    },
  });
}

export function useCreateServiceBundle() {
  return useBundlesMutation(
    (slug, body: ServiceBundleCreateRequest) =>
      serviceBundlesService.create(slug, body),
    "service_bundles.toast.created",
  );
}

export function useUpdateServiceBundle() {
  return useBundlesMutation(
    (slug, { uuid, body }: { uuid: string; body: ServiceBundlePatchRequest }) =>
      serviceBundlesService.update(slug, uuid, body),
    "service_bundles.toast.updated",
  );
}

export function useDeleteServiceBundle() {
  return useBundlesMutation(
    (slug, uuid: string) => serviceBundlesService.remove(slug, uuid),
    "service_bundles.toast.deleted",
  );
}

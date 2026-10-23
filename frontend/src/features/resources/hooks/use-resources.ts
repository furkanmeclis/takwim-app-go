"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { resourcesKeys } from "@/features/resources/hooks/query-keys";
import { resourcesService } from "@/features/resources/services/resources.service";
import type {
  ClosureCreateRequest,
  ClosurePatchRequest,
  ListResourcesParams,
  ResourceCreateRequest,
  ResourcePatchRequest,
} from "@/features/resources/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useResourcesList(params: ListResourcesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: resourcesKeys.list(slug, params),
    queryFn: () => resourcesService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useResourcesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: resourcesKeys.meta(slug),
    queryFn: () => resourcesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useResourcesFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: resourcesKeys.facets(slug),
    queryFn: () => resourcesService.facets(slug),
    staleTime: 60_000,
  });
}

export function useResource(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: resourcesKeys.detail(slug, uuid ?? ""),
    queryFn: () => resourcesService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useResourceClosures(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...resourcesKeys.closures(slug, uuid), params],
    queryFn: () => resourcesService.listClosures(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useResourceActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...resourcesKeys.activity(slug, uuid), params],
    queryFn: () => resourcesService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateResources() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: resourcesKeys.all(slug) });
}

export function useCreateResource() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateResources();
  return useMutation({
    mutationFn: (body: ResourceCreateRequest) =>
      resourcesService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("resources.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateResource() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateResources();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: ResourcePatchRequest;
    }) => resourcesService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("resources.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteResource() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateResources();
  return useMutation({
    mutationFn: (uuid: string) => resourcesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("resources.toast.deleted"));
      void invalidate();
    },
  });
}

export function useSaveClosure(resourceUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateResources();
  return useMutation({
    mutationFn: ({
      closureUuid,
      body,
    }: {
      closureUuid?: string;
      body: ClosureCreateRequest | ClosurePatchRequest;
    }) =>
      closureUuid
        ? resourcesService.updateClosure(slug, resourceUuid, closureUuid, body)
        : resourcesService.createClosure(
            slug,
            resourceUuid,
            body as ClosureCreateRequest,
          ),
    onSuccess: (_data, variables) => {
      appToast.success(
        t(
          variables.closureUuid
            ? "resources.closures.toast.updated"
            : "resources.closures.toast.created",
        ),
      );
      void invalidate();
    },
  });
}

export function useDeleteClosure(resourceUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateResources();
  return useMutation({
    mutationFn: (closureUuid: string) =>
      resourcesService.removeClosure(slug, resourceUuid, closureUuid),
    onSuccess: () => {
      appToast.success(t("resources.closures.toast.deleted"));
      void invalidate();
    },
  });
}

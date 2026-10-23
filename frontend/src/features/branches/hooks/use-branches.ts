"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { branchesKeys } from "@/features/branches/hooks/query-keys";
import { branchesService } from "@/features/branches/services/branches.service";
import type {
  BranchCreateRequest,
  BranchPatchRequest,
  ListBranchesParams,
  ServicePointCreateRequest,
  ServicePointPatchRequest,
} from "@/features/branches/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useBranchesList(params: ListBranchesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: branchesKeys.list(slug, params),
    queryFn: () => branchesService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useBranchesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: branchesKeys.meta(slug),
    queryFn: () => branchesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useBranchFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: branchesKeys.facets(slug),
    queryFn: () => branchesService.facets(slug),
    staleTime: 60_000,
  });
}

export function useBranch(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: branchesKeys.detail(slug, uuid ?? ""),
    queryFn: () => branchesService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useServicePoints(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: branchesKeys.servicePoints(slug, uuid),
    queryFn: () => branchesService.listServicePoints(slug, uuid),
  });
}

export function useBranchActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...branchesKeys.activity(slug, uuid), params],
    queryFn: () => branchesService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateBranches() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: branchesKeys.all(slug) });
}

export function useCreateBranch() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: (body: BranchCreateRequest) =>
      branchesService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("branches.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateBranch() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: BranchPatchRequest }) =>
      branchesService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("branches.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteBranch() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: (uuid: string) => branchesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("branches.toast.deleted"));
      void invalidate();
    },
  });
}

export function useSaveServicePoint(branchUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: ({
      pointUuid,
      body,
    }: {
      pointUuid?: string;
      body: ServicePointCreateRequest | ServicePointPatchRequest;
    }) =>
      pointUuid
        ? branchesService.updateServicePoint(slug, branchUuid, pointUuid, body)
        : branchesService.createServicePoint(
            slug,
            branchUuid,
            body as ServicePointCreateRequest,
          ),
    onSuccess: (_data, variables) => {
      appToast.success(
        t(
          variables.pointUuid
            ? "branches.service_points.toast.updated"
            : "branches.service_points.toast.created",
        ),
      );
      void invalidate();
    },
  });
}

export function useDeleteServicePoint(branchUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: (pointUuid: string) =>
      branchesService.removeServicePoint(slug, branchUuid, pointUuid),
    onSuccess: () => {
      appToast.success(t("branches.service_points.toast.deleted"));
      void invalidate();
    },
  });
}

export function useUploadBranchImage(branchUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: (file: File) =>
      branchesService.uploadImage(slug, branchUuid, file),
    onSuccess: () => {
      appToast.success(t("branches.gallery.toast.uploaded"));
      void invalidate();
    },
  });
}

export function useReorderBranchImages(branchUuid: string) {
  const { slug } = useTenant();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: (imageUuids: string[]) =>
      branchesService.reorderImages(slug, branchUuid, imageUuids),
    onSuccess: () => void invalidate(),
  });
}

export function useDeleteBranchImage(branchUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateBranches();
  return useMutation({
    mutationFn: (imageUuid: string) =>
      branchesService.removeImage(slug, branchUuid, imageUuid),
    onSuccess: () => {
      appToast.success(t("branches.gallery.toast.deleted"));
      void invalidate();
    },
  });
}

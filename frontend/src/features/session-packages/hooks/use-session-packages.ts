"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { cashKeys } from "@/features/cash/hooks/query-keys";
import { sessionPackagesKeys } from "@/features/session-packages/hooks/query-keys";
import { sessionPackagesService } from "@/features/session-packages/services/session-packages.service";
import type {
  CustomerPackageRefundRequest,
  CustomerPackageSellRequest,
  ListCustomerPackagesParams,
  ListSessionPackagesParams,
  PackagePeriod,
  SessionPackageInput,
} from "@/features/session-packages/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useSessionPackages(
  params: ListSessionPackagesParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: sessionPackagesKeys.packages(slug, params),
    queryFn: () => sessionPackagesService.listPackages(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCustomerPackages(
  params: ListCustomerPackagesParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: sessionPackagesKeys.sold(slug, params),
    queryFn: () => sessionPackagesService.listSold(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCustomerPackage(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: sessionPackagesKeys.soldDetail(slug, uuid),
    queryFn: () => sessionPackagesService.getSold(slug, uuid),
  });
}

/** Engine capabilities of the definitions (bulk actions, TEC-984). */
export function useSessionPackagesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: sessionPackagesKeys.packagesMeta(slug),
    queryFn: () => sessionPackagesService.packagesMeta(slug),
    staleTime: 60_000,
  });
}

/** Engine capabilities of the sold packages (export, TEC-984). */
export function useCustomerPackagesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...sessionPackagesKeys.all(slug), "sold-meta"],
    queryFn: () => sessionPackagesService.soldMeta(slug),
    staleTime: 60_000,
  });
}

export function useCustomerPackageFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: sessionPackagesKeys.soldFacets(slug),
    queryFn: () => sessionPackagesService.soldFacets(slug),
    staleTime: 30_000,
  });
}

export function usePackageSummary(period: PackagePeriod) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: sessionPackagesKeys.summary(slug, period),
    queryFn: () => sessionPackagesService.summary(slug, period),
    placeholderData: (previous) => previous,
  });
}

/** A write changes the package lists, balances and the cash book. */
function useInvalidatePackages() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: sessionPackagesKeys.all(slug),
      }),
      queryClient.invalidateQueries({ queryKey: cashKeys.all(slug) }),
    ]);
}

export function useSaveSessionPackage() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePackages();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid?: string;
      body: SessionPackageInput;
    }) =>
      uuid
        ? sessionPackagesService.updatePackage(slug, uuid, body)
        : sessionPackagesService.createPackage(slug, body),
    onSuccess: (_pkg, { uuid }) => {
      appToast.success(
        t(
          uuid
            ? "session_packages.toast.updated"
            : "session_packages.toast.created",
        ),
      );
      void invalidate();
    },
  });
}

export function useDeleteSessionPackage() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePackages();
  return useMutation({
    mutationFn: (uuid: string) =>
      sessionPackagesService.deletePackage(slug, uuid),
    onSuccess: () => {
      appToast.success(t("session_packages.toast.deleted"));
      void invalidate();
    },
  });
}

export function useSellPackage() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePackages();
  return useMutation({
    mutationFn: (body: CustomerPackageSellRequest) =>
      sessionPackagesService.sell(slug, body),
    onSuccess: () => {
      appToast.success(t("session_packages.toast.sold"));
      void invalidate();
    },
  });
}

export function useRefundPackage(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePackages();
  return useMutation({
    mutationFn: (body: CustomerPackageRefundRequest) =>
      sessionPackagesService.refund(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("session_packages.toast.refunded"));
      void invalidate();
    },
  });
}

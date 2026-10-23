"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { customersKeys } from "@/features/customers/hooks/query-keys";
import { customersService } from "@/features/customers/services/customers.service";
import type {
  CustomerCreateRequest,
  CustomerPatchRequest,
  ListCustomersParams,
} from "@/features/customers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useCustomersList(params: ListCustomersParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customersKeys.list(slug, params),
    queryFn: () => customersService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCustomersMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customersKeys.meta(slug),
    queryFn: () => customersService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useCustomersFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customersKeys.facets(slug),
    queryFn: () => customersService.facets(slug),
    staleTime: 60_000,
  });
}

export function useCustomer(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customersKeys.detail(slug, uuid ?? ""),
    queryFn: () => customersService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useCustomerActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...customersKeys.activity(slug, uuid), params],
    queryFn: () => customersService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateCustomers() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: customersKeys.all(slug) });
}

export function useCreateCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (body: CustomerCreateRequest) =>
      customersService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("customers.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CustomerPatchRequest;
    }) => customersService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("customers.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (uuid: string) => customersService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("customers.toast.deleted"));
      void invalidate();
    },
  });
}

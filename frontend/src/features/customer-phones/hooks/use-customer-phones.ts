"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  customerPhonesService,
  type ListCustomerPhoneReleasesParams,
} from "@/features/customer-phones/services/customer-phones.service";
import type { CustomerPhoneReassignmentInput } from "@/features/customer-phones/types";
import { useAppMutation } from "@/lib/query/mutation";

export const customerPhonesKeys = {
  all: ["platform", "customer-phones"] as const,
  lists: () => [...customerPhonesKeys.all, "list"] as const,
  list: (params: ListCustomerPhoneReleasesParams) =>
    [...customerPhonesKeys.lists(), params] as const,
};

export function useCustomerPhoneReleases(
  params: ListCustomerPhoneReleasesParams,
) {
  return useQuery({
    queryKey: customerPhonesKeys.list(params),
    queryFn: () => customerPhonesService.list(params),
  });
}

export function useReassignCustomerPhone() {
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: CustomerPhoneReassignmentInput) =>
      customerPhonesService.reassign(body),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: customerPhonesKeys.lists(),
      });
    },
  });
}

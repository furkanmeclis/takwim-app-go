"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { customerPanelService } from "@/features/customer-panel/services/customer-panel.service";
import type { CustomerListParams } from "@/features/customer-panel/types";

export const customerPanelKeys = {
  all: ["customer-panel"] as const,
  me: () => [...customerPanelKeys.all, "me"] as const,
  reservations: (params: CustomerListParams) =>
    [...customerPanelKeys.all, "reservations", params] as const,
  reservation: (uuid: string) =>
    [...customerPanelKeys.all, "reservation", uuid] as const,
  availability: (uuid: string, date: string) =>
    [...customerPanelKeys.all, "availability", uuid, date] as const,
  payments: (params: CustomerListParams) =>
    [...customerPanelKeys.all, "payments", params] as const,
  paymentSummary: () =>
    [...customerPanelKeys.all, "payments", "summary"] as const,
};

/** The signed-in person; a 401 means there is no customer session. */
export function useCustomerProfile() {
  return useQuery({
    queryKey: customerPanelKeys.me(),
    queryFn: customerPanelService.me,
    retry: false,
    staleTime: 30_000,
  });
}

/** Visits (`money` false) or the payments view of them (`money` true). */
export function useCustomerList(money: boolean, params: CustomerListParams) {
  return useQuery({
    queryKey: money
      ? customerPanelKeys.payments(params)
      : customerPanelKeys.reservations(params),
    queryFn: () =>
      money
        ? customerPanelService.payments(params)
        : customerPanelService.reservations(params),
    placeholderData: keepPreviousData,
    retry: false,
  });
}

export function useCustomerReservation(uuid: string) {
  return useQuery({
    queryKey: customerPanelKeys.reservation(uuid),
    queryFn: () => customerPanelService.reservation(uuid),
    retry: false,
  });
}

/** Free starts of one day for the visit's services (online channel). */
export function useCustomerAvailability(
  uuid: string,
  date: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: customerPanelKeys.availability(uuid, date),
    queryFn: () => customerPanelService.availability(uuid, date, date),
    enabled: enabled && Boolean(date),
    retry: false,
  });
}

export function useCustomerPaymentSummary() {
  return useQuery({
    queryKey: customerPanelKeys.paymentSummary(),
    queryFn: customerPanelService.paymentSummary,
    retry: false,
  });
}

export function useStartSignIn() {
  return useMutation({
    mutationFn: (phone: string) => customerPanelService.start(phone),
  });
}

export function useVerifySignIn() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ phone, code }: { phone: string; code: string }) =>
      customerPanelService.verify(phone, code),
    onSuccess: (session) => {
      queryClient.removeQueries({ queryKey: customerPanelKeys.all });
      queryClient.setQueryData(customerPanelKeys.me(), session.profile);
    },
  });
}

export function useCustomerLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (everywhere: boolean) =>
      customerPanelService.logout(everywhere),
    onSettled: () => {
      queryClient.removeQueries({ queryKey: customerPanelKeys.all });
    },
  });
}

/** After a change the detail, the lists and the payments are refreshed. */
function useRefreshAfterChange() {
  const queryClient = useQueryClient();
  return (uuid: string) => {
    void queryClient.invalidateQueries({
      queryKey: [...customerPanelKeys.all, "reservations"],
    });
    void queryClient.invalidateQueries({
      queryKey: [...customerPanelKeys.all, "payments"],
    });
    void queryClient.invalidateQueries({
      queryKey: [...customerPanelKeys.all, "availability", uuid],
    });
  };
}

export function useCancelReservation(uuid: string) {
  const queryClient = useQueryClient();
  const refresh = useRefreshAfterChange();
  return useMutation({
    mutationFn: (version: number) => customerPanelService.cancel(uuid, version),
    onSuccess: (reservation) => {
      queryClient.setQueryData(
        customerPanelKeys.reservation(uuid),
        reservation,
      );
      refresh(uuid);
    },
  });
}

export function useRescheduleReservation(uuid: string) {
  const queryClient = useQueryClient();
  const refresh = useRefreshAfterChange();
  return useMutation({
    mutationFn: ({
      startsAt,
      version,
    }: {
      startsAt: string;
      version: number;
    }) => customerPanelService.reschedule(uuid, startsAt, version),
    onSuccess: (reservation) => {
      queryClient.setQueryData(
        customerPanelKeys.reservation(uuid),
        reservation,
      );
      refresh(uuid);
    },
  });
}

/** Reloads the detail (after a conflict the page shows the current state). */
export function useReloadReservation(uuid: string) {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: customerPanelKeys.reservation(uuid),
    });
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { customerNotificationsService } from "@/features/customer-notifications/services/customer-notifications.service";
import type {
  CustomerNotificationPreviewInput,
  CustomerNotificationSaveInput,
  ListCustomerNotificationDeliveriesParams,
} from "@/features/customer-notifications/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const customerNotificationKeys = {
  all: (slug: string) => ["tenant", slug, "customer-notifications"] as const,
  settings: (slug: string) =>
    [...customerNotificationKeys.all(slug), "settings"] as const,
  deliveries: (
    slug: string,
    params: ListCustomerNotificationDeliveriesParams,
  ) => [...customerNotificationKeys.all(slug), "deliveries", params] as const,
};

export function useCustomerNotificationSettings() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerNotificationKeys.settings(slug),
    queryFn: () => customerNotificationsService.settings(slug),
  });
}

function useInvalidateSettings() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: customerNotificationKeys.settings(slug),
    });
}

export function useSaveCustomerNotification() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateSettings();
  return useMutation({
    mutationFn: (body: CustomerNotificationSaveInput) =>
      customerNotificationsService.save(slug, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("customer_notifications.toast.saved"));
      void invalidate();
    },
  });
}

export function useResetCustomerNotification() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateSettings();
  return useMutation({
    mutationFn: (event: string) =>
      customerNotificationsService.reset(slug, event),
    onSuccess: () => {
      appToast.success(t("customer_notifications.toast.reset"));
      void invalidate();
    },
  });
}

export function usePreviewCustomerNotification() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: CustomerNotificationPreviewInput) =>
      customerNotificationsService.preview(slug, body),
  });
}

export function useCustomerNotificationDeliveries(
  params: ListCustomerNotificationDeliveriesParams,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerNotificationKeys.deliveries(slug, params),
    queryFn: () => customerNotificationsService.deliveries(slug, params),
    placeholderData: (previous) => previous,
  });
}

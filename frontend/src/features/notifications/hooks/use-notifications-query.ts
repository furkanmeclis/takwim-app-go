"use client";

import { useQuery } from "@tanstack/react-query";

import {
  NOTIFICATION_INBOX_CHANNEL,
  NOTIFICATION_INBOX_LIMIT,
} from "@/features/notifications/constants";
import { notificationsKeys } from "@/features/notifications/hooks/query-keys";
import { useApiScope } from "@/features/tenant/hooks/use-api-scope";
import { scopeKey } from "@/lib/api/scoped-request";
import {
  notificationsService,
  type ListPlatformNotificationsParams,
} from "@/features/notifications/services/notifications.service";

export function usePlatformNotificationsList(
  params: ListPlatformNotificationsParams,
  enabled = true,
) {
  return useQuery({
    queryKey: notificationsKeys.platformList(params),
    queryFn: () => notificationsService.listPlatform(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function usePlatformNotificationsMeta(enabled = true) {
  return useQuery({
    queryKey: notificationsKeys.platformMeta(),
    queryFn: () => notificationsService.platformMeta(),
    enabled,
    staleTime: 5 * 60_000,
  });
}

export function useNotification(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: notificationsKeys.detail(uuid ?? ""),
    queryFn: () => notificationsService.get(uuid!),
    enabled: Boolean(uuid) && enabled,
  });
}

/**
 * Personal inbox: actor-scoped in-app notifications of the current shell
 * (organization panel: that organization + personal; platform: personal).
 */
export function useNotificationsInbox(enabled = true) {
  const scope = useApiScope();
  return useQuery({
    queryKey: notificationsKeys.inboxFor(scopeKey(scope)),
    queryFn: () =>
      notificationsService.listInbox(scope, {
        limit: NOTIFICATION_INBOX_LIMIT,
        offset: 0,
        sort: "-created_at",
        channel: NOTIFICATION_INBOX_CHANNEL,
      }),
    enabled,
    placeholderData: (previous) => previous,
    refetchOnWindowFocus: true,
  });
}

/** Personal inbox unread badge of the current shell. */
export function useNotificationsUnreadCount(enabled = true) {
  const scope = useApiScope();
  return useQuery({
    queryKey: notificationsKeys.unreadCountFor(scopeKey(scope)),
    queryFn: () => notificationsService.unreadCount(scope),
    enabled,
    placeholderData: (previous) => previous,
    refetchOnWindowFocus: true,
  });
}

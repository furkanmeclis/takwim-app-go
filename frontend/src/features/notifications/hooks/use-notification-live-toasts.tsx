"use client";

import { realtimeConfig } from "@/config/realtime";
import { isLiveSurfaceChannel } from "@/features/notifications/constants";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import {
  enrichLiveNotification,
  isLiveNotificationInScope,
  liveNotificationFromMessage,
} from "@/features/notifications/lib/live-notification";
import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";
import { useNotificationLiveStore } from "@/features/notifications/lib/notification-live-store";
import { useRealtimeEvent } from "@/hooks/use-realtime";
import { RealtimeEvents } from "@/lib/realtime/events";

const LIVE_EVENT_TYPES = new Set<string>([
  RealtimeEvents.NotificationItemCreated,
]);

/**
 * Centrifugo live notifications land in the corner host.
 * Inbox / table stay in-app; realtime is the on-screen channel.
 */
export function useNotificationLiveToasts(enabled = true) {
  const active = enabled && realtimeConfig.enabled;
  const push = useNotificationLiveStore((state) => state.push);
  // Organization panel: its own notifications + personal ones; platform
  // shell: personal ones only (same scope as the inbox).
  const organizationUuid = useOptionalTenant()?.organization?.uuid ?? null;

  useNotificationRealtimeInvalidate(enabled);

  useRealtimeEvent(
    "*",
    (message) => {
      if (!LIVE_EVENT_TYPES.has(message.type)) return;
      const incoming = liveNotificationFromMessage(message);
      if (!incoming || !isLiveSurfaceChannel(incoming.channel)) return;
      if (!isLiveNotificationInScope(incoming, organizationUuid)) return;

      void (async () => {
        const notification = await enrichLiveNotification(incoming);
        push(notification);
      })();
    },
    { enabled: active },
  );
}

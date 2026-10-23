import { realtimeConfig } from "@/config/realtime";

/**
 * Channel name builders — never hardcode channel strings in features.
 */
export const RealtimeChannels = {
  systemNotifications: () => realtimeConfig.channels.systemNotifications,
  user: (userId: number | string) => realtimeConfig.channels.user(userId),
  resource: (resource: string) => realtimeConfig.channels.resource(resource),
  organizationResource: (organizationUuid: string, resource: string) =>
    realtimeConfig.channels.organizationResource(organizationUuid, resource),
} as const;

export type RealtimeChannelBuilder = typeof RealtimeChannels;

export function isAuthorizedChannelShape(channel: string): boolean {
  if (channel === realtimeConfig.channels.systemNotifications) return true;
  if (/^user:[0-9a-fA-F-]+$/.test(channel)) return true;
  if (/^user:\d+$/.test(channel)) return true;
  if (/^resource:[a-z0-9._-]+$/.test(channel)) return true;
  if (/^org:[0-9a-fA-F-]+:resource:[a-z0-9._-]+$/.test(channel)) return true;
  return false;
}

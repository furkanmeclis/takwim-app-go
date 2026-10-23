/**
 * Centrifugo realtime — infrastructure kept; disable/stub if upstream unavailable.
 */
export const realtimeConfig = {
  wsUrl:
    process.env.NEXT_PUBLIC_CENTRIFUGO_URL ??
    process.env.NEXT_PUBLIC_WS_URL ??
    "ws://127.0.0.1:8000/connection/websocket",
  reconnect: {
    minDelayMs: 500,
    maxDelayMs: 10_000,
  },
  /** Roles that may subscribe to system notification channels. */
  platformRoles: ["super_admin"] as const,
  channels: {
    systemNotifications: "system.notifications",
    user: (userId: number | string) => `user:${userId}`,
    /** Live list changes of one platform resource (`platform.users`, …). */
    resource: (resource: string) => `resource:${resource}`,
    /** Live list changes of one tenant resource inside an organization. */
    organizationResource: (organizationUuid: string, resource: string) =>
      `org:${organizationUuid}:resource:${resource}`,
  },
  presenceEnabled: true,
  /** When false, RealtimeProvider stays idle (no connect attempts). */
  enabled: process.env.NEXT_PUBLIC_REALTIME_ENABLED !== "false",
} as const;

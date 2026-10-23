import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";
import type { ApiScope } from "@/lib/api/scoped-request";
import { tenantRequest } from "@/lib/api/tenant-request";
import type { ServerListParams } from "@/components/entity";

export type Notification = components["schemas"]["Notification"];
export type NotificationPage = components["schemas"]["NotificationPage"];
export type UnreadCount = components["schemas"]["UnreadCount"];
export type ResourceMeta = components["schemas"]["ResourceMeta"];

export type NotificationListResult = NotificationPage;

export type ListPlatformNotificationsParams = ServerListParams & {
  status?: string;
  channel?: string;
  scope?: "me" | "all";
  user_uuid?: string;
};

export type ListInboxNotificationsParams = ServerListParams & {
  status?: string;
  channel?: string;
  unread?: boolean;
};

export const notificationsService = {
  async listPlatform(params: ListPlatformNotificationsParams) {
    return unwrap<NotificationListResult>(
      await apiClient.GET("/v1/platform/notifications", {
        params: {
          // Backend accepts status/channel; OpenAPI list params omit them.
          query: {
            limit: params.limit,
            offset: params.offset,
            sort: params.sort,
            q: params.q,
            ...(params.status ? { status: params.status } : {}),
            ...(params.channel ? { channel: params.channel } : {}),
            ...(params.scope ? { scope: params.scope } : {}),
            ...(params.user_uuid ? { user_uuid: params.user_uuid } : {}),
          } as {
            limit?: number;
            offset?: number;
            sort?: string;
            q?: string;
            status?: string;
            channel?: string;
            scope?: "me" | "all";
            user_uuid?: string;
          },
        },
      }),
    );
  },

  async platformMeta() {
    return unwrap<ResourceMeta>(
      await apiClient.GET("/v1/platform/notifications/meta"),
    );
  },

  /**
   * Personal inbox of the current shell. Inside `/t/{slug}` the tenant route
   * lists that organization's notifications plus personal ones; the platform
   * / account shell lists personal (un-stamped) notifications only.
   */
  async listInbox(scope: ApiScope, params: ListInboxNotificationsParams) {
    if (scope.kind === "tenant") {
      return tenantRequest<NotificationListResult>(
        scope.slug,
        "GET",
        "/v1/tenant/notifications",
        {
          query: {
            limit: params.limit,
            offset: params.offset,
            sort: params.sort,
            q: params.q,
            status: params.status,
            channel: params.channel,
            unread:
              params.unread === undefined ? undefined : String(params.unread),
          },
        },
      );
    }
    return unwrap<NotificationListResult>(
      await apiClient.GET("/v1/notifications", {
        params: {
          query: {
            limit: params.limit,
            offset: params.offset,
            sort: params.sort,
            q: params.q,
            status: params.status,
            channel: params.channel,
            unread: params.unread,
          },
        },
      }),
    );
  },

  async inboxMeta() {
    return unwrap<ResourceMeta>(await apiClient.GET("/v1/notifications/meta"));
  },

  async unreadCount(scope: ApiScope) {
    if (scope.kind === "tenant") {
      return tenantRequest<UnreadCount>(
        scope.slug,
        "GET",
        "/v1/tenant/notifications/unread-count",
      );
    }
    return unwrap<UnreadCount>(
      await apiClient.GET("/v1/notifications/unread-count"),
    );
  },

  async get(uuid: string) {
    return unwrap<Notification>(
      await apiClient.GET("/v1/notifications/{uuid}", {
        params: { path: { uuid } },
      }),
    );
  },

  async markRead(uuid: string) {
    return unwrap<Notification>(
      await apiClient.POST("/v1/notifications/{uuid}/read", {
        params: { path: { uuid } },
      }),
    );
  },

  async markAllRead(scope: ApiScope) {
    if (scope.kind === "tenant") {
      return tenantRequest<{ status: string }>(
        scope.slug,
        "POST",
        "/v1/tenant/notifications/read-all",
      );
    }
    return unwrap<{ status: string }>(
      await apiClient.POST("/v1/notifications/read-all"),
    );
  },

  async redeemSignedAction(source: {
    uuid?: string;
    signed_action_url?: string | null;
  }) {
    try {
      const first = await redeemParsed(source.signed_action_url);
      if (first) return first;
    } catch {
      // Expired or stale signature — refresh from the API.
    }
    if (!source.uuid) return null;
    const full = await notificationsService.get(source.uuid);
    return redeemParsed(full.signed_action_url);
  },
};

async function redeemParsed(signed: string | null | undefined) {
  const parsed = parseSignedActionUrl(signed);
  if (!parsed) return null;
  return unwrap<{
    destination: string;
    notification: Notification;
  }>(
    await apiClient.GET("/v1/notifications/{uuid}/action", {
      params: {
        path: { uuid: parsed.uuid },
        query: { exp: parsed.exp, sig: parsed.sig },
      },
    }),
  );
}

function parseSignedActionUrl(href?: string | null) {
  if (!href) return null;
  try {
    const url = new URL(href, "https://action.invalid");
    const parts = url.pathname.split("/").filter(Boolean);
    if (
      parts.length < 4 ||
      parts[0] !== "v1" ||
      parts[1] !== "notifications" ||
      parts[3] !== "action"
    ) {
      return null;
    }
    const exp = Number(url.searchParams.get("exp"));
    const sig = url.searchParams.get("sig") ?? "";
    if (!Number.isFinite(exp) || !sig) return null;
    return { uuid: parts[2], exp, sig };
  } catch {
    return null;
  }
}

import { routes } from "@/config/routes";

export type NotificationActionKind = "download" | "route" | "external";

export type NotificationAction = {
  kind: NotificationActionKind;
  href: string;
  labelKey: "notifications.actions.download" | "notifications.actions.open";
};

const EXPORT_DOWNLOAD =
  /^\/v1\/platform\/exports\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/download\/?$/i;
const PLATFORM_API_ITEM =
  /^\/v1\/platform\/([a-z0-9-]+)\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:\/.*)?$/i;

function platformRouteFor(resource: string, uuid: string): string | null {
  switch (resource) {
    case "exports":
      return routes.platform.exports.detail(uuid);
    case "imports":
      return routes.platform.imports.detail(uuid);
    case "notifications":
      return routes.platform.notifications.detail(uuid);
    case "users":
      return routes.platform.users.detail(uuid);
    case "roles":
      return routes.platform.roles.detail(uuid);
    default:
      return `/platform/${resource}/${uuid}`;
  }
}

/** Primary + optional companion actions for a notification `action_url`. */
export function notificationActions(
  actionUrl?: string | null,
): NotificationAction[] {
  if (!actionUrl) return [];

  const download = EXPORT_DOWNLOAD.exec(actionUrl);
  if (download?.[1]) {
    return [
      {
        kind: "download",
        href: actionUrl,
        labelKey: "notifications.actions.download",
      },
      {
        kind: "route",
        href: routes.platform.exports.detail(download[1]),
        labelKey: "notifications.actions.open",
      },
    ];
  }

  if (actionUrl.startsWith("/v1/platform/")) {
    const item = PLATFORM_API_ITEM.exec(actionUrl);
    if (item?.[1] && item[2]) {
      const href = platformRouteFor(item[1], item[2]);
      if (href) {
        return [
          { kind: "route", href, labelKey: "notifications.actions.open" },
        ];
      }
    }
    return [];
  }

  if (actionUrl.startsWith("/")) {
    return [
      {
        kind: "route",
        href: actionUrl,
        labelKey: "notifications.actions.open",
      },
    ];
  }

  if (/^https?:\/\//i.test(actionUrl)) {
    return [
      {
        kind: "external",
        href: actionUrl,
        labelKey: "notifications.actions.open",
      },
    ];
  }

  return [];
}

export function notificationFingerprint(input: {
  uuid?: string | null;
  template_code?: string | null;
  title?: string | null;
  body?: string | null;
  action_url?: string | null;
}) {
  if (input.uuid) return input.uuid;
  const templateKey = [input.template_code, input.action_url]
    .map((value) => (value ?? "").trim())
    .filter(Boolean)
    .join("|");
  if (templateKey) return templateKey;
  return [input.template_code, input.title, input.body]
    .map((value) => (value ?? "").trim())
    .filter(Boolean)
    .join("|");
}

import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { tenantDownloadFile } from "@/lib/api/tenant-request";
import {
  tenantExportDownload,
  type NotificationAction,
} from "@/features/notifications/lib/notification-action";
import { notificationsService } from "@/features/notifications/services/notifications.service";

export type NotificationActionSource = {
  uuid?: string;
  signed_action_url?: string | null;
};

type RunNotificationActionOptions = {
  action: NotificationAction;
  navigate: (href: string) => void;
  notification?: NotificationActionSource | null;
};

export async function runNotificationAction({
  action,
  navigate,
  notification,
}: RunNotificationActionOptions) {
  if (notification) {
    await notificationsService.redeemSignedAction(notification);
  }
  if (action.kind === "download") {
    // Tenant export: one click downloads through the tenant API with the
    // organization of the link (the page at the same URL does the same for
    // push notifications and deep links).
    const tenant = tenantExportDownload(action.href);
    const { blob, filename } = tenant
      ? await tenantDownloadFile(
          tenant.slug,
          `/v1/tenant/exports/${tenant.uuid}/download`,
        )
      : await platformDownloadFile(action.href);
    triggerBrowserDownload(blob, filename ?? "export");
    return;
  }
  if (action.kind === "external") {
    window.open(action.href, "_blank", "noopener,noreferrer");
    return;
  }
  navigate(action.href);
}

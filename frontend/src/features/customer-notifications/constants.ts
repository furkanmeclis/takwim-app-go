import type {
  NotificationChannelName,
  NotificationDeliveryStatus,
  NotificationEventCode,
} from "@/features/customer-notifications/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Display order of the events (the API answers in the same order). */
export const NOTIFICATION_EVENTS: NotificationEventCode[] = [
  "reservation.requested",
  "reservation.accepted",
  "reservation.rejected",
  "reservation.rescheduled",
  "reservation.cancelled",
  "reservation.reminder",
];

export const NOTIFICATION_CHANNELS: NotificationChannelName[] = [
  "sms",
  "whatsapp",
  "email",
];

export const NOTIFICATION_STATUSES: NotificationDeliveryStatus[] = [
  "sent",
  "delivered",
  "sending",
  "failed",
  "skipped",
];

export const EVENT_LABEL_KEYS: Record<NotificationEventCode, string> = {
  "reservation.requested": "customer_notifications.events.requested",
  "reservation.accepted": "customer_notifications.events.accepted",
  "reservation.rejected": "customer_notifications.events.rejected",
  "reservation.rescheduled": "customer_notifications.events.rescheduled",
  "reservation.cancelled": "customer_notifications.events.cancelled",
  "reservation.reminder": "customer_notifications.events.reminder",
};

export const EVENT_HINT_KEYS: Record<NotificationEventCode, string> = {
  "reservation.requested": "customer_notifications.event_hints.requested",
  "reservation.accepted": "customer_notifications.event_hints.accepted",
  "reservation.rejected": "customer_notifications.event_hints.rejected",
  "reservation.rescheduled": "customer_notifications.event_hints.rescheduled",
  "reservation.cancelled": "customer_notifications.event_hints.cancelled",
  "reservation.reminder": "customer_notifications.event_hints.reminder",
};

export const CHANNEL_LABEL_KEYS: Record<NotificationChannelName, string> = {
  sms: "customer_notifications.channels.sms",
  whatsapp: "customer_notifications.channels.whatsapp",
  email: "customer_notifications.channels.email",
};

export const STATUS_LABEL_KEYS: Record<NotificationDeliveryStatus, string> = {
  sending: "customer_notifications.statuses.sending",
  sent: "customer_notifications.statuses.sent",
  delivered: "customer_notifications.statuses.delivered",
  failed: "customer_notifications.statuses.failed",
  skipped: "customer_notifications.statuses.skipped",
};

export const STATUS_TONES: Record<
  NotificationDeliveryStatus,
  "default" | "success" | "warning" | "danger"
> = {
  sending: "default",
  sent: "success",
  delivered: "success",
  failed: "danger",
  skipped: "warning",
};

export const VARIABLE_LABEL_KEYS: Record<string, string> = {
  customer_name: "customer_notifications.variables.customer_name",
  business_name: "customer_notifications.variables.business_name",
  branch_name: "customer_notifications.variables.branch_name",
  reference: "customer_notifications.variables.reference",
  starts_at: "customer_notifications.variables.starts_at",
  date: "customer_notifications.variables.date",
  time: "customer_notifications.variables.time",
  services: "customer_notifications.variables.services",
  link: "customer_notifications.variables.link",
};

/** Reminder offsets offered in the editor, in minutes before the visit. */
export const REMINDER_OFFSET_PRESETS = [
  15, 30, 60, 120, 180, 360, 720, 1440, 2880, 4320, 10080,
];
export const MAX_REMINDER_OFFSETS = 3;
export const MAX_BODY = 1000;
export const MAX_SUBJECT = 200;

export function eventLabel(t: Translate, code: string) {
  const key = EVENT_LABEL_KEYS[code as NotificationEventCode];
  return key ? t(key) : code;
}

export function channelLabel(t: Translate, name: string) {
  const key = CHANNEL_LABEL_KEYS[name as NotificationChannelName];
  return key ? t(key) : name;
}

export function statusLabel(t: Translate, status: string) {
  const key = STATUS_LABEL_KEYS[status as NotificationDeliveryStatus];
  return key ? t(key) : status;
}

/** "1 gün önce", "2 saat önce", "30 dakika önce". */
export function offsetLabel(t: Translate, minutes: number) {
  if (minutes % 1440 === 0) {
    const count = minutes / 1440;
    return count === 1
      ? t("customer_notifications.offset.days_one", { count })
      : t("customer_notifications.offset.days_other", { count });
  }
  if (minutes % 60 === 0) {
    const count = minutes / 60;
    return count === 1
      ? t("customer_notifications.offset.hours_one", { count })
      : t("customer_notifications.offset.hours_other", { count });
  }
  return minutes === 1
    ? t("customer_notifications.offset.minutes_one", { count: minutes })
    : t("customer_notifications.offset.minutes_other", { count: minutes });
}

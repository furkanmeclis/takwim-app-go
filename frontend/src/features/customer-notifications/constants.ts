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
  "refund.completed",
  "refund.processing",
];

/**
 * Events written by other modules with their own settings (not on the
 * events tab): the confirmation request of the confirmation add-on.
 */
export const MANAGED_EVENT_LABEL_KEYS: Record<string, string> = {
  "confirmation.request": "customer_notifications.events.confirmation_request",
};

/** Events of the delivery history filter. */
export const HISTORY_EVENTS: string[] = [
  ...NOTIFICATION_EVENTS,
  ...Object.keys(MANAGED_EVENT_LABEL_KEYS),
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
  "refund.completed": "customer_notifications.events.refund_completed",
  "refund.processing": "customer_notifications.events.refund_processing",
};

export const EVENT_HINT_KEYS: Record<NotificationEventCode, string> = {
  "reservation.requested": "customer_notifications.event_hints.requested",
  "reservation.accepted": "customer_notifications.event_hints.accepted",
  "reservation.rejected": "customer_notifications.event_hints.rejected",
  "reservation.rescheduled": "customer_notifications.event_hints.rescheduled",
  "reservation.cancelled": "customer_notifications.event_hints.cancelled",
  "reservation.reminder": "customer_notifications.event_hints.reminder",
  "refund.completed": "customer_notifications.event_hints.refund_completed",
  "refund.processing": "customer_notifications.event_hints.refund_processing",
};

/** Labels of every channel (the voice call only carries confirmations). */
export const CHANNEL_LABEL_KEYS: Record<
  NotificationChannelName | "voice_call",
  string
> = {
  sms: "customer_notifications.channels.sms",
  whatsapp: "customer_notifications.channels.whatsapp",
  email: "customer_notifications.channels.email",
  voice_call: "customer_notifications.channels.voice_call",
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

/**
 * Errors the engine writes itself (provider errors are shown as they come).
 * "outcome unknown": the send never got its result (TEC-974), it is not
 * sent again.
 */
export const ERROR_LABEL_KEYS: Record<string, string> = {
  "outcome unknown": "customer_notifications.errors.outcome_unknown",
  "no delivery report": "customer_notifications.errors.no_delivery_report",
  "provider not configured":
    "customer_notifications.errors.provider_not_configured",
  "provider not configured (logged only)":
    "customer_notifications.errors.provider_not_configured",
  "no phone number": "customer_notifications.errors.no_phone_number",
  "no e-mail address": "customer_notifications.errors.no_email_address",
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
  deadline: "customer_notifications.variables.deadline",
};

/** Reminder offsets offered in the editor, in minutes before the visit. */
export const REMINDER_OFFSET_PRESETS = [
  15, 30, 60, 120, 180, 360, 720, 1440, 2880, 4320, 10080,
];
export const MAX_REMINDER_OFFSETS = 3;
export const MAX_BODY = 1000;
export const MAX_SUBJECT = 200;

export function eventLabel(t: Translate, code: string) {
  const key =
    EVENT_LABEL_KEYS[code as NotificationEventCode] ??
    MANAGED_EVENT_LABEL_KEYS[code];
  return key ? t(key) : code;
}

export function channelLabel(t: Translate, name: string) {
  const key = CHANNEL_LABEL_KEYS[name as keyof typeof CHANNEL_LABEL_KEYS];
  return key ? t(key) : name;
}

export function statusLabel(t: Translate, status: string) {
  const key = STATUS_LABEL_KEYS[status as NotificationDeliveryStatus];
  return key ? t(key) : status;
}

export function errorLabel(t: Translate, error: string) {
  const key = ERROR_LABEL_KEYS[error];
  return key ? t(key) : error;
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

import type {
  ConfirmationState,
  ConfirmationTask,
} from "@/features/confirmations/types";
import { dateKeyInZone } from "@/lib/utils/format";

export type Tone = "default" | "success" | "warning" | "danger";

/** List states in the order a desk handles them, with their chip tone. */
export const STATES: Record<ConfirmationState, { key: string; tone: Tone }> = {
  change_requested: {
    key: "confirmations.list.states.change_requested",
    tone: "warning",
  },
  unreachable: { key: "confirmations.list.states.unreachable", tone: "danger" },
  flagged: { key: "confirmations.list.states.flagged", tone: "danger" },
  pending: { key: "confirmations.list.states.pending", tone: "warning" },
  scheduled: { key: "confirmations.list.states.scheduled", tone: "default" },
  confirmed: { key: "confirmations.list.states.confirmed", tone: "success" },
  declined: { key: "confirmations.list.states.declined", tone: "danger" },
  cancelled: { key: "confirmations.list.states.cancelled", tone: "danger" },
  dropped: { key: "confirmations.list.states.dropped", tone: "default" },
};

/** Results of a request / a channel attempt. */
export const SEND_STATUS: Record<string, { key: string; tone: Tone }> = {
  pending: { key: "confirmations.attempts.status.pending", tone: "default" },
  sending: { key: "confirmations.attempts.status.sending", tone: "default" },
  sent: { key: "confirmations.attempts.status.sent", tone: "success" },
  delivered: {
    key: "confirmations.attempts.status.delivered",
    tone: "success",
  },
  failed: { key: "confirmations.attempts.status.failed", tone: "danger" },
  skipped: { key: "confirmations.attempts.status.skipped", tone: "warning" },
  cancelled: {
    key: "confirmations.attempts.status.cancelled",
    tone: "default",
  },
};

/** Where an answer came from (`answer_channel`). */
export const ANSWER_SOURCES: Record<string, string> = {
  panel: "reservations.confirmation_task.sources.panel",
  visit_link: "reservations.confirmation_task.sources.visit_link",
  phone: "reservations.confirmation_task.sources.phone",
};

/** The customer's answers. */
export const ANSWER_KEYS: Record<string, string> = {
  confirm: "reservations.confirmation_task.answers.confirm",
  cancel: "reservations.confirmation_task.answers.cancel",
  change: "reservations.confirmation_task.answers.change",
};

/** Periods of the list toolbar. */
export const PERIODS = ["today", "tomorrow", "upcoming", "all"] as const;
export type Period = (typeof PERIODS)[number];

/** The instant a local calendar date ("2026-10-24") starts in a zone. */
export function zonedDayStart(dateKey: string, timeZone: string) {
  const guess = Date.parse(`${dateKey}T00:00:00Z`);
  const local = dateKeyInZone(new Date(guess), timeZone);
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(guess));
  const hour = Number(parts.find((p) => p.type === "hour")?.value ?? 0);
  const minute = Number(parts.find((p) => p.type === "minute")?.value ?? 0);
  const shown = Date.parse(`${local}T00:00:00Z`) + (hour * 60 + minute) * 60000;
  return new Date(guess - (shown - guess));
}

function addDaysKey(dateKey: string, days: number) {
  const d = new Date(`${dateKey}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** `starts_from` / `starts_to` of a period, fixed at `now`. */
export function periodRange(
  period: Period,
  now: Date,
  timeZone: string,
): { starts_from?: string; starts_to?: string } {
  const today = dateKeyInZone(now, timeZone);
  switch (period) {
    case "today":
      return {
        starts_from: zonedDayStart(today, timeZone).toISOString(),
        starts_to: zonedDayStart(addDaysKey(today, 1), timeZone).toISOString(),
      };
    case "tomorrow":
      return {
        starts_from: zonedDayStart(
          addDaysKey(today, 1),
          timeZone,
        ).toISOString(),
        starts_to: zonedDayStart(addDaysKey(today, 2), timeZone).toISOString(),
      };
    case "upcoming":
      return { starts_from: now.toISOString() };
    default:
      return {};
  }
}

/** `tel:` link of a customer phone (E.164). */
export function telHref(phone: string | null | undefined) {
  return phone ? `tel:${phone.replace(/[^\d+]/g, "")}` : null;
}

/** The task can still be moved or cancelled by the business. */
export function isActiveVisit(row: Pick<ConfirmationTask, "status">) {
  return row.status === "active";
}

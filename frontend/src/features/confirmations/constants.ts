import { offsetLabel } from "@/features/customer-notifications/constants";
import type {
  ConfirmationRule,
  OnNoAnswer,
} from "@/features/confirmations/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Minutes before the visit offered for the first request. */
export const FIRST_REQUEST_PRESETS = [
  60, 120, 180, 360, 720, 1440, 2880, 4320, 10080,
];
/** Minutes before the visit offered for the follow-up (0 = none). */
export const FOLLOW_UP_PRESETS = [0, 30, 60, 120, 180, 360, 720, 1440, 2880];
/** Minutes offered between a missed call and the next one. */
export const CALL_RETRY_PRESETS = [15, 30, 45, 60, 90, 120, 180, 240];

/** Minutes before the visit offered for the last answer (0 = the start). */
export const DEADLINE_PRESETS = [0, 30, 60, 120, 180, 360, 720, 1440];

export const ON_NO_ANSWER_KEYS: Record<OnNoAnswer, string> = {
  cancel: "confirmations.on_no_answer.cancel",
  flag: "confirmations.on_no_answer.flag",
};

export const ON_NO_ANSWER_HINT_KEYS: Record<OnNoAnswer, string> = {
  cancel: "confirmations.on_no_answer.cancel_hint",
  flag: "confirmations.on_no_answer.flag_hint",
};

/** Minute of the day as "09:00" ("24:00" for the end of the day). */
export function clock(minute: number) {
  const h = Math.floor(minute / 60);
  const m = minute % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

/** Presets plus the saved value, sorted, as select options. */
export function minuteOptions(
  presets: number[],
  current: number,
  label: (minutes: number) => string,
) {
  const values = new Set<number>(presets);
  values.add(current);
  return [...values]
    .sort((a, b) => a - b)
    .map((value) => ({ value: String(value), label: label(value) }));
}

export function followUpLabel(t: Translate, minutes: number) {
  return minutes === 0
    ? t("confirmations.timing.no_follow_up")
    : offsetLabel(t, minutes);
}

export function deadlineLabel(t: Translate, minutes: number) {
  return minutes === 0
    ? t("confirmations.timing.until_start")
    : offsetLabel(t, minutes);
}

export function windowLabel(t: Translate, rule: ConfirmationRule) {
  if (rule.window_start_minute == null || rule.window_end_minute == null) {
    return t("confirmations.timing.any_time");
  }
  return t("confirmations.timing.window_value", {
    start: clock(rule.window_start_minute),
    end: clock(rule.window_end_minute),
  });
}

/** The hours a call may ring (the request hours when the rule has none). */
export function callWindowLabel(t: Translate, rule: ConfirmationRule) {
  if (
    rule.call_window_start_minute == null ||
    rule.call_window_end_minute == null
  ) {
    return t("confirmations.calls.same_as_window", {
      window: windowLabel(t, rule),
    });
  }
  return t("confirmations.timing.window_value", {
    start: clock(rule.call_window_start_minute),
    end: clock(rule.call_window_end_minute),
  });
}

/** "1 tekrar arama, 30 dakika sonra" or "Tekrar aranmaz". */
export function callRetriesLabel(t: Translate, rule: ConfirmationRule) {
  if (rule.call_retries === 0) return t("confirmations.calls.no_retry");
  return t("confirmations.calls.retries_value", {
    count: rule.call_retries,
    minutes: rule.call_retry_minutes,
  });
}

/** "24 saat önce · 2 istek · son yanıt 2 saat önce". */
export function timingSummary(t: Translate, rule: ConfirmationRule) {
  return [
    offsetLabel(t, rule.first_request_minutes),
    t("confirmations.timing.attempts_value", { count: rule.max_attempts }),
    t("confirmations.timing.deadline_value", {
      when: deadlineLabel(t, rule.deadline_minutes),
    }),
  ].join(" · ");
}

import { isApiError } from "@/lib/api";

type Translate = (key: string, params?: Record<string, string>) => string;

/** Why the API refuses a cancel or a reschedule (`actions.*.reason`). */
export const REASON_KEYS: Record<string, string> = {
  status: "customer_panel.reasons.status",
  cutoff: "customer_panel.reasons.cutoff",
  business_unavailable: "customer_panel.reasons.business_unavailable",
  online_unavailable: "customer_panel.reasons.online_unavailable",
  services_changed: "customer_panel.reasons.services_changed",
};

const CONFLICT_KEYS: Record<string, string> = {
  taken: "customer_panel.errors.taken",
  stale: "customer_panel.errors.stale",
  invalid_transition: "customer_panel.errors.invalid_transition",
};

/**
 * A person-facing message for a failed panel call. The API decides what is
 * allowed; the browser only explains its answer.
 */
export function customerErrorMessage(t: Translate, error: unknown): string {
  if (!isApiError(error)) return t("customer_panel.errors.generic");
  const detail = error.details[0]?.code ?? "";
  switch (error.code) {
    case "OTP_INVALID":
      return t("customer_panel.login.invalid_code");
    case "ACTION_NOT_ALLOWED":
      return t(REASON_KEYS[detail] ?? "customer_panel.errors.generic");
    case "RATE_LIMITED":
      return t("customer_panel.errors.rate_limited");
    default:
      break;
  }
  if (error.status === 409 && CONFLICT_KEYS[detail]) {
    return t(CONFLICT_KEYS[detail]);
  }
  if (error.status === 404) return t("customer_panel.detail.not_found");
  if (error.status === 429) return t("customer_panel.errors.rate_limited");
  if (error.isValidation) {
    const field = error.details[0]?.field;
    if (field === "phone") return t("customer_panel.login.invalid_phone");
    if (field === "code") return t("customer_panel.login.code_format");
  }
  return t("customer_panel.errors.generic");
}

/** True when the visit changed meanwhile; the page reloads it. */
export function isConflict(error: unknown) {
  return isApiError(error) && error.status === 409;
}

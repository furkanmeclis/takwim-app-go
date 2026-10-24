import { ApiError, emitApiError, isApiError } from "@/lib/api";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/**
 * Field messages of the timetable API by `field.code`, then by `code`
 * alone. Unknown details keep the API's own message.
 */
const DETAIL_KEYS: Record<string, string> = {
  "branch_uuid.not_found": "classes.errors.branch_not_found",
  "service_uuid.not_found": "classes.errors.service_not_found",
  "service_uuid.not_group": "classes.errors.not_group",
  "service_uuid.not_offered": "classes.errors.not_offered",
  "staff_uuid.not_found": "classes.errors.staff_not_found",
  "staff_uuid.not_allowed": "classes.errors.staff_not_allowed",
  "staff_uuid.not_qualified": "classes.errors.not_qualified",
  "staff_uuid.not_in_branch": "classes.errors.staff_not_in_branch",
  "staff_uuid.staff_busy": "classes.errors.staff_busy",
  "resource_uuid.not_found": "classes.errors.resource_not_found",
  "resource_uuid.not_in_branch": "classes.errors.resource_not_in_branch",
  "resource_uuid.kind_mismatch": "classes.errors.kind_mismatch",
  "resource_uuid.resource_busy": "classes.errors.resource_busy",
  "start_time.duplicate": "classes.errors.duplicate",
  "valid_until.before_from": "classes.errors.before_from",
  "sessions.has_bookings": "classes.errors.has_bookings",
  "capacity.below_booked": "classes.errors.below_booked",
  "date.past": "classes.errors.past",
  required: "classes.errors.required",
  invalid: "classes.errors.invalid",
  too_long: "classes.errors.too_long",
};

function detailKey(field?: string, code?: string) {
  if (!code) return undefined;
  return DETAIL_KEYS[`${field ?? ""}.${code}`] ?? DETAIL_KEYS[code];
}

/**
 * The same error with its field messages in the panel's language, so the
 * form shows them under the inputs (`AppForm` reads `fieldErrors`).
 */
export function localizeClassError(error: unknown, t: Translate): unknown {
  if (!isApiError(error)) return error;
  return new ApiError({
    status: error.status,
    code: error.code,
    message: error.message,
    requestId: error.requestId,
    body: error.body,
    details: error.details.map((detail) => {
      const key = detailKey(detail.field, detail.code);
      return key ? { ...detail, message: t(key) } : detail;
    }),
  });
}

/**
 * Toasts the error of a timetable write: a known detail in the panel's
 * words, anything else through the global API error toast.
 */
export function reportClassError(error: unknown, t: Translate) {
  if (!isApiError(error)) return;
  for (const detail of error.details) {
    const key = detailKey(detail.field, detail.code);
    if (key) {
      appToast.error(t(key));
      return;
    }
  }
  emitApiError(error);
}

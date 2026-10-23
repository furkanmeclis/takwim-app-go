import { emitApiError, isApiError } from "@/lib/api";
import { appToast } from "@/providers/toast-provider";

/** Conflict details of a status change or a move, said in the panel's words. */
const DETAIL_KEYS: Record<string, string> = {
  other_staff: "reservations.errors.other_staff",
  taken: "reservations.errors.taken",
  stale: "reservations.errors.stale",
  invalid_transition: "reservations.errors.invalid_transition",
};

/**
 * Toasts the error of an action or a move: a known conflict in the panel's
 * words, anything else through the global API error toast.
 */
export function reportReservationError(
  error: unknown,
  t: (key: string) => string,
) {
  if (!isApiError(error)) return;
  const known = error.details.find((d) => d.code && DETAIL_KEYS[d.code]);
  if (known?.code) {
    appToast.error(t(DETAIL_KEYS[known.code]));
    return;
  }
  emitApiError(error);
}

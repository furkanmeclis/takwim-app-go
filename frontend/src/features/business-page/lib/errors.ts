import { emitApiError, isApiError } from "@/lib/api";
import { PublicBookingError } from "@/features/business-page/services/public-booking.service";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** True when the picked time went to someone else (`409 starts_at / taken`). */
export function isSlotTaken(error: unknown) {
  return (
    isApiError(error) &&
    error.status === 409 &&
    error.details.some((d) => d.code === "taken")
  );
}

/**
 * Toasts a booking error in the customer's words: a taken time, closed
 * bookings and rate limits (with the wait of `Retry-After`); anything else
 * through the global API error toast.
 */
export function reportBookingError(error: unknown, t: Translate) {
  if (!isApiError(error)) {
    appToast.error(t("business_page.errors.generic"));
    return;
  }
  if (isSlotTaken(error)) {
    appToast.error(t("business_page.errors.taken"));
    return;
  }
  if (error.code === "BOOKING_UNAVAILABLE") {
    appToast.error(t("business_page.errors.unavailable"));
    return;
  }
  if (error.status === 429) {
    const seconds =
      error instanceof PublicBookingError ? error.retryAfter : undefined;
    appToast.error(
      seconds
        ? t("business_page.errors.rate_limited_wait", {
            minutes: Math.max(1, Math.ceil(seconds / 60)),
          })
        : t("business_page.errors.rate_limited"),
    );
    return;
  }
  emitApiError(error);
}

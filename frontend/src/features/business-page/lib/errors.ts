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

/** True when a picked product ran out before the booking (TEC-919). */
export function isOutOfStock(error: unknown) {
  return (
    isApiError(error) &&
    error.details.some((d) => d.code === "insufficient_stock")
  );
}

/** True when the branch took its online bookings of that day (TEC-905). */
export function isDayFull(error: unknown) {
  return (
    isApiError(error) && error.details.some((d) => d.code === "daily_limit")
  );
}

/** True when an answer to the business's questions was refused. */
export function isAnswerError(error: unknown) {
  return (
    isApiError(error) &&
    error.details.some((d) => d.field?.startsWith("custom_fields."))
  );
}

/** True when the vehicle of a car wash booking was refused (TEC-971). */
export function isVehicleError(error: unknown) {
  return (
    isApiError(error) &&
    error.details.some((d) => d.field?.startsWith("vehicle."))
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
  if (isOutOfStock(error)) {
    appToast.error(t("business_page.errors.out_of_stock"));
    return;
  }
  if (isDayFull(error)) {
    appToast.error(t("business_page.errors.day_full"));
    return;
  }
  if (error.details.some((d) => d.code === "open_limit")) {
    appToast.error(t("business_page.errors.open_limit"));
    return;
  }
  if (isAnswerError(error)) {
    appToast.error(t("business_page.errors.answers"));
    return;
  }
  if (isVehicleError(error)) {
    appToast.error(t("business_page.errors.vehicle"));
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

const PAYMENT_ERROR_KEYS: Record<string, string> = {
  PAYMENT_NOT_AVAILABLE: "business_page.errors.payment_unavailable",
  PAYMENT_NOT_READY: "business_page.errors.payment_not_ready",
  PAYMENT_PROVIDER_ERROR: "business_page.errors.payment_provider",
};

/** Toasts why the payment page did not open (else as a booking error). */
export function reportPaymentError(error: unknown, t: Translate) {
  const key = isApiError(error) ? PAYMENT_ERROR_KEYS[error.code] : undefined;
  if (key) {
    appToast.error(t(key));
    return;
  }
  reportBookingError(error, t);
}

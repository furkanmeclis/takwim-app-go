/**
 * Regional defaults (TEC-878). Instants come from the API in UTC; calendar
 * values are shown in the business (or branch) time zone. Amounts are
 * decimal strings ("1300.00") with an ISO 4217 currency next to them.
 */
export const DEFAULT_TIMEZONE = "Europe/Istanbul";
export const DEFAULT_CURRENCY = "TRY";

/** Currencies offered in business settings. */
export const CURRENCIES = ["TRY", "EUR", "USD", "GBP"] as const;

/** Every IANA zone the runtime knows, default zone first. */
export function timeZones(): string[] {
  const all =
    typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("timeZone")
      : [DEFAULT_TIMEZONE, "UTC"];
  return [DEFAULT_TIMEZONE, ...all.filter((z) => z !== DEFAULT_TIMEZONE)];
}

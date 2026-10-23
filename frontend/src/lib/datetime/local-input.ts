import { format, isValid, parseISO } from "date-fns";

/**
 * `AppDatePicker withTime` values are local `yyyy-MM-dd'T'HH:mm`; the API
 * takes RFC 3339 instants.
 */
export function localInputToISO(value: string): string | undefined {
  if (!value) return undefined;
  const parsed = parseISO(value);
  return isValid(parsed) ? parsed.toISOString() : undefined;
}

/** API instant → local `yyyy-MM-dd'T'HH:mm` for `AppDatePicker withTime`. */
export function isoToLocalInput(value: string | null | undefined): string {
  if (!value) return "";
  const parsed = parseISO(value);
  return isValid(parsed) ? format(parsed, "yyyy-MM-dd'T'HH:mm") : "";
}

/** Local input `hours` from now, rounded to the next quarter hour. */
export function localInputFromNow(hours: number): string {
  const d = new Date(Date.now() + hours * 3_600_000);
  d.setMinutes(Math.ceil(d.getMinutes() / 15) * 15, 0, 0);
  return format(d, "yyyy-MM-dd'T'HH:mm");
}

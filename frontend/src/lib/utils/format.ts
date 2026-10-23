import {
  format as formatDateFns,
  formatDistanceToNow,
  formatISO,
  parseISO,
  type Locale as DateFnsLocale,
} from "date-fns";
import { enUS as dateFnsEnUS, tr as dateFnsTr } from "date-fns/locale";
import {
  enUS as dayPickerEnUS,
  tr as dayPickerTr,
} from "react-day-picker/locale";

import { i18nConfig, type AppLocale } from "@/config/i18n";
import { DEFAULT_CURRENCY, DEFAULT_TIMEZONE } from "@/config/regional";

export function dateFnsLocale(
  locale: AppLocale = i18nConfig.defaultLocale,
): DateFnsLocale {
  return locale === "tr" ? dateFnsTr : dateFnsEnUS;
}

/** DayPicker locale (`react-day-picker/locale`) for calendar UI. */
export function dayPickerLocale(locale: AppLocale = i18nConfig.defaultLocale) {
  return locale === "tr" ? dayPickerTr : dayPickerEnUS;
}

function dateLocale(locale: AppLocale = i18nConfig.defaultLocale) {
  return dateFnsLocale(locale);
}

export function currency(
  value: number,
  currencyCode = "TRY",
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency: currencyCode,
  }).format(value);
}

export function money(
  value: number,
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function intlLocale(locale: AppLocale) {
  return locale === "tr" ? "tr-TR" : "en-US";
}

/**
 * Parses an API amount ("1300.00", "-0.50") into integer minor units
 * (kuruş). Sums and splits are done on minor units, never on floats.
 * Returns null for anything that is not a decimal with at most 2 digits.
 */
export function toMinor(value: string): number | null {
  const m = /^(-)?(\d+)(?:\.(\d{1,2}))?$/.exec(value.trim());
  if (!m) return null;
  const minor = Number(m[2]) * 100 + Number((m[3] ?? "").padEnd(2, "0"));
  return m[1] ? -minor : minor;
}

/** Renders integer minor units back as the API decimal string. */
export function fromMinor(minor: number): string {
  const sign = minor < 0 ? "-" : "";
  const abs = Math.abs(Math.trunc(minor));
  return `${sign}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
}

/**
 * Splits minor units over weights; every part is rounded toward zero and the
 * remainder goes to the last part, so the parts always add up to the total
 * (same rule as the API's money.Allocate).
 */
export function allocateMinor(total: number, weights: number[]): number[] {
  const sum = weights.reduce((a, w) => a + w, 0);
  if (weights.length === 0 || sum <= 0) return [];
  const parts = weights.map((w) => Math.trunc((total * w) / sum));
  parts[parts.length - 1] =
    total - parts.slice(0, -1).reduce((a, p) => a + p, 0);
  return parts;
}

/** Formats an API amount string with its currency ("1.300,00 ₺"). */
export function amount(
  value: string | null | undefined,
  currencyCode: string = DEFAULT_CURRENCY,
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const minor = value == null ? null : toMinor(value);
  if (minor == null) return "";
  return new Intl.NumberFormat(intlLocale(locale), {
    style: "currency",
    currency: currencyCode,
  }).format(minor / 100);
}

/** Formats a UTC instant in a business / branch time zone. */
export function datetimeInZone(
  value: Date | string,
  timeZone: string = DEFAULT_TIMEZONE,
  locale: AppLocale = i18nConfig.defaultLocale,
  options: Intl.DateTimeFormatOptions = {
    dateStyle: "short",
    timeStyle: "short",
  },
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return new Intl.DateTimeFormat(intlLocale(locale), {
    ...options,
    timeZone,
  }).format(d);
}

/** The calendar date ("2026-06-01") of a UTC instant in a time zone. */
export function dateKeyInZone(
  value: Date | string,
  timeZone: string = DEFAULT_TIMEZONE,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  // en-CA renders ISO order: YYYY-MM-DD.
  return new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(d);
}

export function date(
  value: Date | string,
  pattern = "dd.MM.yyyy",
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return formatDateFns(d, pattern, { locale: dateLocale(locale) });
}

export function datetime(
  value: Date | string,
  pattern = "dd.MM.yyyy HH:mm",
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return formatDateFns(d, pattern, { locale: dateLocale(locale) });
}

export function relativeDatetime(
  value: Date | string,
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return formatDistanceToNow(d, {
    addSuffix: true,
    locale: dateLocale(locale),
  });
}

export function phone(value: string) {
  const digits = value.replace(/\D/g, "");
  if (digits.length === 10) {
    return digits.replace(/(\d{3})(\d{3})(\d{2})(\d{2})/, "$1 $2 $3 $4");
  }
  if (digits.length === 12 && digits.startsWith("90")) {
    return digits.replace(
      /(\d{2})(\d{3})(\d{3})(\d{2})(\d{2})/,
      "+$1 $2 $3 $4 $5",
    );
  }
  return value;
}

export function licensePlate(value: string) {
  return value.trim().toUpperCase().replace(/\s+/g, " ");
}

export function fileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function slug(value: string) {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

export function uuid() {
  return crypto.randomUUID();
}

export function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

export function toIso(value: Date) {
  return formatISO(value);
}

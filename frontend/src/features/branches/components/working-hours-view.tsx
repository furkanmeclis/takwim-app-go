"use client";

import { DAY_KEYS, DAY_LABEL_KEYS } from "@/features/branches/constants";
import type {
  DayHours,
  HolidayException,
  WeeklyHours,
} from "@/features/branches/types";
import { date } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function useDayLabel() {
  const { t } = useLocale();
  return (day: DayHours) =>
    day.closed ? t("branches.hours.closed") : `${day.open} – ${day.close}`;
}

/** Read-only weekly schedule. */
export function WorkingHoursView({ hours }: { hours: WeeklyHours }) {
  const { t } = useLocale();
  const label = useDayLabel();
  return (
    <dl className="divide-border divide-y text-sm">
      {DAY_KEYS.map((key) => (
        <div key={key} className="flex items-center justify-between py-1.5">
          <dt className="text-muted-foreground">{t(DAY_LABEL_KEYS[key])}</dt>
          <dd
            className={cn(
              "tabular-nums",
              hours[key].closed && "text-muted-foreground",
            )}
          >
            {label(hours[key])}
          </dd>
        </div>
      ))}
    </dl>
  );
}

/** Read-only list of dated overrides. */
export function HolidayExceptionsView({
  holidays,
}: {
  holidays: HolidayException[];
}) {
  const { t, locale } = useLocale();
  const label = useDayLabel();
  if (!holidays.length) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("branches.holidays.none")}
      </p>
    );
  }
  return (
    <ul className="divide-border divide-y text-sm">
      {holidays.map((holiday) => (
        <li
          key={holiday.date}
          className="flex flex-wrap items-center justify-between gap-2 py-1.5"
        >
          <span>
            <span className="font-medium tabular-nums">
              {date(holiday.date, undefined, locale)}
            </span>
            {holiday.note ? (
              <span className="text-muted-foreground"> · {holiday.note}</span>
            ) : null}
          </span>
          <span
            className={cn(
              "tabular-nums",
              holiday.closed && "text-muted-foreground",
            )}
          >
            {label(holiday)}
          </span>
        </li>
      ))}
    </ul>
  );
}

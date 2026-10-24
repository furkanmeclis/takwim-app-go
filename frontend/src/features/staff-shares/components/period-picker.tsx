"use client";

import { DatePicker } from "@/components/ui/date-picker";
import { useBusinessToday } from "@/features/cash/lib/format";
import type { StaffSharePeriod } from "@/features/staff-shares/types";
import { useLocale } from "@/providers/locale-provider";

type Period = Required<StaffSharePeriod>;

/**
 * Start and end day of a share period. A cleared day falls back to the
 * first of the month / today, and the end never comes before the start
 * (the API allows at most 366 days).
 */
export function PeriodPicker({
  value,
  onChange,
}: {
  value: Period;
  onChange: (value: Period) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const monthStart = `${today.slice(0, 8)}01`;

  return (
    <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
      <DatePicker
        className="w-full sm:w-40"
        value={value.date_from}
        onChange={(next) => {
          const from = next || monthStart;
          onChange({
            date_from: from,
            date_to: value.date_to < from ? from : value.date_to,
          });
        }}
        placeholder={t("staff_shares.period.from")}
      />
      <DatePicker
        className="w-full sm:w-40"
        value={value.date_to}
        onChange={(next) => {
          const to = next || today;
          onChange({
            date_from: value.date_from > to ? to : value.date_from,
            date_to: to,
          });
        }}
        placeholder={t("staff_shares.period.to")}
      />
    </div>
  );
}

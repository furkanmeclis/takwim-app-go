"use client";

import { addDays, format, parseISO } from "date-fns";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useBusinessToday } from "@/features/cash/lib/format";
import { useReportFilterOptions } from "@/features/reports/hooks/use-reports";
import type { ReportFilter } from "@/features/reports/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const ALL = "__all__";
const HEAD_OFFICE = "head_office";
/** The API allows at most 366 days in one report. */
const MAX_DAYS = 366;

function clampRange(from: string, to: string) {
  const last = format(addDays(parseISO(from), MAX_DAYS - 1), "yyyy-MM-dd");
  return { from, to: to > last ? last : to };
}

type Preset = "today" | "week" | "month" | "last_month";

function presetRange(preset: Preset, today: string) {
  const day = parseISO(today);
  switch (preset) {
    case "today":
      return { from: today, to: today };
    case "week": {
      const weekday = (day.getDay() + 6) % 7; // Monday first
      return {
        from: format(addDays(day, -weekday), "yyyy-MM-dd"),
        to: today,
      };
    }
    case "month":
      return { from: `${today.slice(0, 8)}01`, to: today };
    case "last_month": {
      const firstThis = parseISO(`${today.slice(0, 8)}01`);
      const lastPrev = addDays(firstThis, -1);
      return {
        from: format(lastPrev, "yyyy-MM-01"),
        to: format(lastPrev, "yyyy-MM-dd"),
      };
    }
  }
}

/**
 * Period, branch (or the head office records) and staff of the report. The
 * branch picked in the header still narrows everything; this filter picks
 * inside it.
 */
export function ReportFilters({
  value,
  onChange,
}: {
  value: ReportFilter;
  onChange: (value: ReportFilter) => void;
}) {
  const { t } = useLocale();
  const { branchScope } = useTenant();
  const today = useBusinessToday();
  const options = useReportFilterOptions();

  const setRange = (from: string, to: string) => {
    const range = clampRange(from, to < from ? from : to);
    onChange({ ...value, date_from: range.from, date_to: range.to });
  };

  return (
    <div className="flex w-full flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        {(["today", "week", "month", "last_month"] as const).map((preset) => {
          const range = presetRange(preset, today);
          const active =
            value.date_from === range.from && value.date_to === range.to;
          return (
            <Button
              key={preset}
              type="button"
              size="sm"
              variant={active ? "default" : "outline"}
              onClick={() => setRange(range.from, range.to)}
            >
              {t(`reports.presets.${preset}`)}
            </Button>
          );
        })}
      </div>
      <div className="grid w-full grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
        <DatePicker
          className="w-full"
          value={value.date_from}
          onChange={(next) => setRange(next || today, value.date_to)}
          placeholder={t("reports.filters.date_from")}
        />
        <DatePicker
          className="w-full"
          value={value.date_to}
          onChange={(next) => {
            const to = next || today;
            setRange(value.date_from > to ? to : value.date_from, to);
          }}
          placeholder={t("reports.filters.date_to")}
        />
        {options.branches ? (
          <Select
            value={value.branch || ALL}
            onValueChange={(next) =>
              onChange({ ...value, branch: next === ALL ? undefined : next })
            }
          >
            <SelectTrigger aria-label={t("reports.filters.branch")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>
                {t("reports.filters.all_branches")}
              </SelectItem>
              {options.branches.map((b) => (
                <SelectItem key={b.value} value={b.value}>
                  {b.label}
                </SelectItem>
              ))}
              {branchScope === "all" ? (
                <SelectItem value={HEAD_OFFICE}>
                  {t("reports.head_office")}
                </SelectItem>
              ) : null}
            </SelectContent>
          </Select>
        ) : null}
        {options.staff ? (
          <Select
            value={value.staff || ALL}
            onValueChange={(next) =>
              onChange({ ...value, staff: next === ALL ? undefined : next })
            }
          >
            <SelectTrigger aria-label={t("reports.filters.staff")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>
                {t("reports.filters.all_staff")}
              </SelectItem>
              {options.staff.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
      </div>
    </div>
  );
}

"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type {
  PublicAvailability,
  PublicSlot,
} from "@/features/business-page/types";
import { date as formatDate, datetimeInZone } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const DAY_STATUS_KEYS: Record<string, string> = {
  closed: "business_page.datetime.closed",
  too_far: "business_page.datetime.too_far",
  too_soon: "business_page.datetime.too_soon",
  no_staff: "business_page.datetime.no_staff",
  full: "business_page.datetime.full",
};

/**
 * Day and time step. The strip shows the days of the window the slot
 * engine answered (online channel); a day without free starts is greyed
 * out. The browser never computes availability itself.
 */
export function DatetimeStep({
  query,
  date,
  onDateChange,
  slot,
  onSlotChange,
  canGoBack,
  canGoForward,
  onPrevWindow,
  onNextWindow,
}: {
  query: UseQueryResult<PublicAvailability>;
  /** Picked day ("yyyy-MM-dd"). */
  date: string;
  onDateChange: (date: string) => void;
  slot: PublicSlot | null;
  onSlotChange: (slot: PublicSlot | null) => void;
  canGoBack: boolean;
  canGoForward: boolean;
  onPrevWindow: () => void;
  onNextWindow: () => void;
}) {
  const { t, locale } = useLocale();
  const data = query.data;
  const zone = data?.timezone;
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });
  const day = data?.days.find((d) => d.date === date);
  const first = data?.days[0]?.date;
  const last = data?.days[data.days.length - 1]?.date;

  if (query.isError && !data) {
    return (
      <ErrorState
        title={t("business_page.datetime.load_failed")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <Button
          type="button"
          variant="outline"
          size="icon"
          onClick={onPrevWindow}
          disabled={!canGoBack || query.isFetching}
          aria-label={t("business_page.datetime.previous")}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <p className="flex items-center gap-2 text-sm font-medium">
          {first && last
            ? `${formatDate(first, "d MMM", locale)} – ${formatDate(last, "d MMM", locale)}`
            : "…"}
          {query.isFetching ? (
            <Loader2 className="text-muted-foreground size-4 animate-spin" />
          ) : null}
        </p>
        <Button
          type="button"
          variant="outline"
          size="icon"
          onClick={onNextWindow}
          disabled={!canGoForward || query.isFetching}
          aria-label={t("business_page.datetime.next")}
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>

      {!data ? (
        <div className="flex gap-2 overflow-hidden">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-16 w-14 shrink-0" />
          ))}
        </div>
      ) : (
        <div className="-mx-4 overflow-x-auto px-4 pb-1">
          <ToggleGroup
            type="single"
            variant="outline"
            spacing={2}
            value={date}
            onValueChange={(next) => {
              if (next) onDateChange(next);
            }}
            className="flex w-max gap-2"
            aria-label={t("business_page.datetime.day")}
          >
            {data.days.map((d) => (
              <ToggleGroupItem
                key={d.date}
                value={d.date}
                className={cn(
                  "h-auto w-14 flex-col gap-0 py-2",
                  d.status !== "available" && "text-muted-foreground",
                )}
                aria-label={formatDate(d.date, "PPPP", locale)}
              >
                <span className="text-xs">
                  {formatDate(d.date, "EEE", locale)}
                </span>
                <span className="text-base font-semibold tabular-nums">
                  {formatDate(d.date, "d", locale)}
                </span>
                <span className="text-[10px]">
                  {formatDate(d.date, "MMM", locale)}
                </span>
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>
      )}

      {data && day ? (
        day.slots.length > 0 ? (
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {formatDate(day.date, "PPPP", locale)}
            </p>
            <ToggleGroup
              type="single"
              variant="outline"
              spacing={2}
              value={slot?.starts_at ?? ""}
              onValueChange={(next) =>
                onSlotChange(
                  day.slots.find((s) => s.starts_at === next) ?? null,
                )
              }
              className="grid w-full grid-cols-4 gap-2 sm:grid-cols-6"
              aria-label={t("business_page.datetime.time")}
            >
              {day.slots.map((s) => (
                <ToggleGroupItem
                  key={s.starts_at}
                  value={s.starts_at}
                  className="w-full tabular-nums"
                >
                  {time(s.starts_at)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
        ) : (
          <EmptyState
            className="py-8"
            title={t(
              DAY_STATUS_KEYS[day.status] ?? "business_page.datetime.full",
            )}
            description={t("business_page.datetime.try_other_day")}
          />
        )
      ) : data && data.days.length === 0 ? (
        <EmptyState
          className="py-8"
          title={t("business_page.datetime.none")}
          description={t("business_page.datetime.try_other_day")}
        />
      ) : null}

      {slot && data ? (
        <ul className="bg-muted/40 space-y-1 rounded-md border p-3 text-sm">
          {slot.lines.map((line, index) => (
            <li
              key={`${line.service_uuid}-${index}`}
              className="flex justify-between gap-3"
            >
              <span>
                <span className="tabular-nums">
                  {time(line.starts_at)}–{time(line.ends_at)}
                </span>{" "}
                {data.lines[index]?.service_name ?? ""}
              </span>
              <span className="text-muted-foreground">
                {line.staff?.name ?? ""}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

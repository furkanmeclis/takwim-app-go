"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  Loader2,
  MapPin,
  UserRound,
} from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { windowDays } from "@/features/business-page/lib/wizard";
import type {
  PublicClassSession,
  PublicClassTimetable,
} from "@/features/business-page/types";
import { date as formatDate, datetimeInZone } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Session key: a class and its start. */
export function sessionKey(session: PublicClassSession) {
  return `${session.service.uuid}@${session.starts_at}`;
}

/**
 * Class timetable step (gym, TEC-990): instead of a free time grid the
 * customer picks a session of the class timetable. The day strip shows
 * the days of the window with their bookable sessions; each session shows
 * its time, trainer, studio and free seats, and why it cannot be booked
 * (full, cancelled, started, not yet open). Whether a session books is the
 * API's (`state`), never the browser's.
 */
export function ClassStep({
  query,
  date,
  onDateChange,
  value,
  onChange,
  canGoBack,
  canGoForward,
  onPrevWindow,
  onNextWindow,
  header,
}: {
  query: UseQueryResult<PublicClassTimetable>;
  /** Picked day ("yyyy-MM-dd"). */
  date: string;
  onDateChange: (date: string) => void;
  /** Key of the picked session (`sessionKey`). */
  value: string;
  onChange: (session: PublicClassSession | null) => void;
  canGoBack: boolean;
  canGoForward: boolean;
  onPrevWindow: () => void;
  onNextWindow: () => void;
  /** Shown above the days (the seats to take). */
  header?: ReactNode;
}) {
  const { t, locale } = useLocale();
  const data = query.data;
  const zone = data?.timezone;
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });
  const days = data ? windowDays(data.from, data.to) : [];
  const sessionsOf = (day: string) =>
    data?.sessions.filter((s) => s.date === day) ?? [];
  const daySessions = sessionsOf(date);

  if (query.isError && !data) {
    return (
      <ErrorState
        title={t("business_page.classes.load_failed")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  const stateBadge = (session: PublicClassSession) => {
    if (session.state === "open") {
      return (
        <Badge variant="secondary" className="tabular-nums">
          {t("business_page.seats.left", { count: session.seats_left })}
        </Badge>
      );
    }
    return (
      <Badge variant={session.state === "cancelled" ? "danger" : "outline"}>
        {t(`business_page.classes.state.${session.state}`)}
      </Badge>
    );
  };

  return (
    <div className="space-y-4">
      {header}
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
          {data
            ? `${formatDate(data.from, "d MMM", locale)} – ${formatDate(data.to, "d MMM", locale)}`
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
        <div className="space-y-3">
          <div className="flex gap-2 overflow-hidden">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-16 w-14 shrink-0" />
            ))}
          </div>
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
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
            {days.map((day) => {
              const open = sessionsOf(day).filter(
                (s) => s.state === "open",
              ).length;
              return (
                <ToggleGroupItem
                  key={day}
                  value={day}
                  className={cn(
                    "h-auto w-14 flex-col gap-0 py-2",
                    open === 0 && "text-muted-foreground",
                  )}
                  aria-label={formatDate(day, "PPPP", locale)}
                >
                  <span className="text-xs">
                    {formatDate(day, "EEE", locale)}
                  </span>
                  <span className="text-base font-semibold tabular-nums">
                    {formatDate(day, "d", locale)}
                  </span>
                  <span className="text-[10px]">
                    {open > 0
                      ? t("business_page.classes.count", { count: open })
                      : formatDate(day, "MMM", locale)}
                  </span>
                </ToggleGroupItem>
              );
            })}
          </ToggleGroup>
        </div>
      )}

      {data && date ? (
        daySessions.length > 0 ? (
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {formatDate(date, "PPPP", locale)}
            </p>
            <RadioGroup
              value={value}
              onValueChange={(next) =>
                onChange(
                  daySessions.find(
                    (s) => sessionKey(s) === next && s.state === "open",
                  ) ?? null,
                )
              }
              aria-label={t("business_page.classes.sessions")}
            >
              {daySessions.map((session) => {
                const key = sessionKey(session);
                const id = `session-${key}`;
                const closed = session.state !== "open";
                return (
                  <ChoiceCard
                    key={key}
                    id={id}
                    disabled={closed}
                    className={cn(closed && "opacity-70")}
                    title={
                      <span className="flex flex-wrap items-center gap-x-2">
                        <span className="tabular-nums">
                          {time(session.starts_at)}–{time(session.ends_at)}
                        </span>
                        <span
                          className={cn(
                            session.state === "cancelled" && "line-through",
                          )}
                        >
                          {session.service.name}
                        </span>
                      </span>
                    }
                    description={
                      <>
                        <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                          <span className="inline-flex items-center gap-1">
                            <UserRound className="size-3.5 shrink-0" />
                            {session.trainer?.name ??
                              t("business_page.classes.trainer_assigned")}
                          </span>
                          {session.area ? (
                            <span className="inline-flex items-center gap-1">
                              <MapPin className="size-3.5 shrink-0" />
                              {session.area}
                            </span>
                          ) : null}
                        </span>
                        {session.trainer_changed ? (
                          <span className="block">
                            {t("business_page.classes.trainer_changed")}
                          </span>
                        ) : null}
                        {session.capacity_changed &&
                        session.state !== "cancelled" ? (
                          <span className="block">
                            {t("business_page.classes.capacity_changed", {
                              count: session.capacity,
                            })}
                          </span>
                        ) : null}
                      </>
                    }
                    aside={stateBadge(session)}
                    control={
                      <RadioGroupItem value={key} id={id} disabled={closed} />
                    }
                  />
                );
              })}
            </RadioGroup>
          </div>
        ) : (
          <EmptyState
            className="py-8"
            title={t("business_page.classes.none_day")}
            description={t("business_page.datetime.try_other_day")}
          />
        )
      ) : data && data.sessions.length === 0 ? (
        <EmptyState
          className="py-8"
          title={t("business_page.classes.none")}
          description={t("business_page.classes.none_description")}
        />
      ) : null}
    </div>
  );
}

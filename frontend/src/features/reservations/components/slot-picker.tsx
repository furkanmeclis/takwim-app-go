"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { DatePicker } from "@/components/ui/date-picker";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type {
  Availability,
  AvailabilitySlot,
} from "@/features/reservations/types";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DAY_STATUS_KEYS: Record<string, string> = {
  closed: "reservations.slots.closed",
  too_far: "reservations.slots.too_far",
  too_soon: "reservations.slots.too_soon",
  no_class: "reservations.slots.no_class",
  no_staff: "reservations.slots.no_staff",
  full: "reservations.slots.full",
};

type SlotPickerProps = {
  /** Availability of the picked day (`useAvailability`). */
  query: UseQueryResult<Availability>;
  /** Branch, services and day are known (the query runs). */
  ready: boolean;
  date: string;
  onDateChange: (date: string) => void;
  /** ISO start of the picked slot. */
  value: string;
  onChange: (slot: AvailabilitySlot | null) => void;
  error?: string;
};

/**
 * Day + free starts from the API's slot engine (panel channel, 15 minute
 * steps). The browser never computes availability: it shows the answer and
 * the staff / resources the engine assigned to each line.
 */
export function SlotPicker({
  query,
  ready,
  date,
  onDateChange,
  value,
  onChange,
  error,
}: SlotPickerProps) {
  const { t, locale } = useLocale();
  const day = query.data?.days[0];
  const zone = query.data?.timezone;
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });
  const picked = day?.slots.find((slot) => slot.starts_at === value) ?? null;
  // Group class (gym): free seats of the session at that start.
  const seatsLeft = (slot: AvailabilitySlot) =>
    slot.lines.find((line) => line.seats_left != null)?.seats_left ?? null;

  return (
    <div className="space-y-3">
      <DatePicker value={date} onChange={onDateChange} />
      {!ready ? (
        <p className="text-muted-foreground text-sm">
          {t("reservations.slots.pick_first")}
        </p>
      ) : query.isLoading ? (
        <p className="text-muted-foreground flex items-center gap-2 text-sm">
          <Loader2 className="size-4 animate-spin" />
          {t("reservations.slots.loading")}
        </p>
      ) : query.isError ? (
        <ErrorState
          title={t("reservations.slots.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : day && day.slots.length > 0 ? (
        <ToggleGroup
          type="single"
          variant="outline"
          spacing={2}
          value={value}
          onValueChange={(next) =>
            onChange(day.slots.find((slot) => slot.starts_at === next) ?? null)
          }
          className="flex flex-wrap justify-start gap-2"
          aria-invalid={Boolean(error)}
          aria-label={t("reservations.fields.slot")}
        >
          {day.slots.map((slot) => (
            <ToggleGroupItem
              key={slot.starts_at}
              value={slot.starts_at}
              className="h-auto min-w-16 flex-col gap-0 py-1.5 tabular-nums"
            >
              {time(slot.starts_at)}
              {seatsLeft(slot) != null ? (
                <span className="text-muted-foreground text-xs font-normal">
                  {t("reservations.slots.seats_left", {
                    count: seatsLeft(slot) ?? 0,
                  })}
                </span>
              ) : null}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      ) : (
        <EmptyState
          className="py-8"
          title={t(
            DAY_STATUS_KEYS[day?.status ?? "full"] ?? "reservations.slots.full",
          )}
          description={t("reservations.slots.try_other_day")}
        />
      )}
      {picked ? (
        <ul className="bg-muted/40 space-y-1 rounded-md border p-3 text-sm">
          {picked.lines.map((line, index) => {
            const service = query.data?.lines[index]?.service_name ?? "";
            const who = [
              line.staff?.name,
              ...line.resources.map((resource) => resource.name),
              line.seats_left != null
                ? t("reservations.slots.seats_left", {
                    count: line.seats_left,
                  })
                : null,
            ]
              .filter(Boolean)
              .join(" · ");
            return (
              <li
                key={`${line.service_uuid}-${index}`}
                className="flex justify-between gap-3"
              >
                <span>
                  <span className="tabular-nums">
                    {time(line.starts_at)}–{time(line.ends_at)}
                  </span>{" "}
                  {service}
                </span>
                <span className="text-muted-foreground">{who || "—"}</span>
              </li>
            );
          })}
        </ul>
      ) : null}
      {error ? <p className="text-destructive text-sm">{error}</p> : null}
    </div>
  );
}

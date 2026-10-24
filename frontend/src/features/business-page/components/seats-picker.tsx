"use client";

import { Minus, Plus, Users } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Seats of a group class (gym, TEC-971): how many places the booking
 * takes in one session, 1 by default, at most the class's capacity. The
 * day step lists only the sessions with that many free seats (API).
 */
export function SeatsPicker({
  value,
  capacity,
  onChange,
  disabled,
}: {
  value: number;
  capacity: number;
  onChange: (value: number) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  return (
    <div className="bg-muted/40 flex flex-wrap items-center justify-between gap-3 rounded-md border p-3">
      <div className="flex min-w-0 items-center gap-2">
        <Users className="text-muted-foreground size-4 shrink-0" />
        <div className="min-w-0">
          <p className="text-sm font-medium">
            {t("business_page.seats.title")}
          </p>
          <p className="text-muted-foreground text-xs">
            {t("business_page.seats.hint", { capacity })}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="icon"
          onClick={() => onChange(Math.max(1, value - 1))}
          disabled={disabled || value <= 1}
          aria-label={t("business_page.seats.decrease")}
        >
          <Minus className="size-4" />
        </Button>
        <span
          className="w-8 text-center text-base font-semibold tabular-nums"
          aria-live="polite"
        >
          {value}
        </span>
        <Button
          type="button"
          variant="outline"
          size="icon"
          onClick={() => onChange(Math.min(capacity, value + 1))}
          disabled={disabled || value >= capacity}
          aria-label={t("business_page.seats.increase")}
        >
          <Plus className="size-4" />
        </Button>
      </div>
    </div>
  );
}

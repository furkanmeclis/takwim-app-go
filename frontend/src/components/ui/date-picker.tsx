"use client";

import { format, isValid, parseISO } from "date-fns";
import { CalendarIcon } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { dateFnsLocale, dayPickerLocale } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type DatePickerProps = {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  /** Display pattern for the trigger label. Defaults to locale-aware `PPP`. */
  displayFormat?: string;
  captionLayout?: React.ComponentProps<typeof Calendar>["captionLayout"];
  align?: React.ComponentProps<typeof PopoverContent>["align"];
  "aria-invalid"?: boolean;
  /**
   * Date and time: the value becomes local `yyyy-MM-dd'T'HH:mm` and a time
   * field appears under the calendar (callers convert to ISO for the API).
   */
  withTime?: boolean;
  /** Time used when a day is picked before any time (withTime). */
  defaultTime?: string;
};

const TIME_RE = /^\d{2}:\d{2}$/;

function parseDateValue(value: string | undefined): Date | undefined {
  if (!value) return undefined;
  const parsed = parseISO(value);
  return isValid(parsed) ? parsed : undefined;
}

function timeOf(value: string | undefined): string {
  const time = value?.split("T")[1]?.slice(0, 5) ?? "";
  return TIME_RE.test(time) ? time : "";
}

/**
 * Shared date picker (Popover + Calendar).
 * Value is `yyyy-MM-dd` (empty string when cleared), or local
 * `yyyy-MM-dd'T'HH:mm` with `withTime`.
 * Calendar labels follow the active app locale (`tr` / `en`).
 */
export function DatePicker({
  id,
  value,
  onChange,
  placeholder,
  disabled,
  className,
  displayFormat,
  captionLayout,
  align = "start",
  "aria-invalid": ariaInvalid,
  withTime = false,
  defaultTime = "09:00",
}: DatePickerProps) {
  const { locale, t } = useLocale();
  const selected = parseDateValue(value);
  const dfLocale = dateFnsLocale(locale);
  const time = timeOf(value) || defaultTime;
  const pattern = displayFormat ?? (withTime ? "PPP HH:mm" : "PPP");

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          disabled={disabled}
          aria-invalid={ariaInvalid}
          data-empty={!selected}
          className={cn(
            "data-[empty=true]:text-muted-foreground h-9 w-full justify-start gap-2 px-3 font-normal",
            className,
          )}
        >
          <CalendarIcon className="text-muted-foreground size-4" />
          {selected
            ? format(selected, pattern, { locale: dfLocale })
            : (placeholder ?? t("form.pick_date"))}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={align}>
        <Calendar
          mode="single"
          locale={dayPickerLocale(locale)}
          selected={selected}
          onSelect={(date) => {
            const day = date ? format(date, "yyyy-MM-dd") : "";
            onChange?.(day && withTime ? `${day}T${time}` : day);
          }}
          defaultMonth={selected}
          captionLayout={captionLayout}
        />
        {withTime ? (
          <div className="border-t p-3">
            <Input
              type="time"
              aria-label={t("form.pick_time")}
              value={time}
              disabled={disabled || !selected}
              onChange={(event) => {
                const next = event.target.value;
                if (!selected || !TIME_RE.test(next)) return;
                onChange?.(`${format(selected, "yyyy-MM-dd")}T${next}`);
              }}
            />
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}

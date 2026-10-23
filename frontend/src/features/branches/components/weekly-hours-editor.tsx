"use client";

import { Controller, get, useFormContext } from "react-hook-form";

import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { DAY_KEYS, DAY_LABEL_KEYS } from "@/features/branches/constants";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type WeeklyHoursEditorProps = {
  /** Form path of a WeeklyHours object, e.g. `working_hours`. */
  name: string;
  disabled?: boolean;
};

/**
 * Seven-row weekly schedule editor bound to react-hook-form: per day an
 * open/closed switch and opening / closing times (24h).
 */
export function WeeklyHoursEditor({ name, disabled }: WeeklyHoursEditorProps) {
  const { t } = useLocale();
  const {
    control,
    register,
    watch,
    formState: { errors },
  } = useFormContext();

  return (
    <div className="divide-border border-border divide-y rounded-lg border">
      {DAY_KEYS.map((day) => {
        const base = `${name}.${day}`;
        const closed = Boolean(watch(`${base}.closed`));
        const openError = get(errors, `${base}.open`)?.message as
          string | undefined;
        const closeError = get(errors, `${base}.close`)?.message as
          string | undefined;
        return (
          <div
            key={day}
            className="grid grid-cols-[minmax(6rem,1fr)_auto] items-start gap-3 px-3 py-2.5 sm:grid-cols-[8rem_9rem_1fr]"
          >
            <span className="pt-2 text-sm font-medium">
              {t(DAY_LABEL_KEYS[day])}
            </span>
            <Controller
              control={control}
              name={`${base}.closed`}
              render={({ field }) => (
                <label className="flex items-center gap-2 pt-1.5 text-sm">
                  <Switch
                    checked={!field.value}
                    onCheckedChange={(open) => field.onChange(!open)}
                    disabled={disabled}
                    aria-label={t(DAY_LABEL_KEYS[day])}
                  />
                  <span className={cn(closed && "text-muted-foreground")}>
                    {closed
                      ? t("branches.hours.closed")
                      : t("branches.hours.open")}
                  </span>
                </label>
              )}
            />
            <div className="col-span-2 flex flex-wrap items-start gap-2 sm:col-span-1">
              {closed ? (
                <span className="text-muted-foreground pt-2 text-sm">
                  {t("branches.hours.closed_all_day")}
                </span>
              ) : (
                <>
                  <div className="space-y-1">
                    <Input
                      type="time"
                      step={60}
                      className={cn("w-32", openError && "border-destructive")}
                      aria-label={t("branches.hours.opens")}
                      aria-invalid={Boolean(openError)}
                      disabled={disabled}
                      {...register(`${base}.open`)}
                    />
                    {openError ? (
                      <p className="text-destructive text-xs">{openError}</p>
                    ) : null}
                  </div>
                  <span className="text-muted-foreground pt-2 text-sm">–</span>
                  <div className="space-y-1">
                    <Input
                      type="time"
                      step={60}
                      className={cn("w-32", closeError && "border-destructive")}
                      aria-label={t("branches.hours.closes")}
                      aria-invalid={Boolean(closeError)}
                      disabled={disabled}
                      {...register(`${base}.close`)}
                    />
                    {closeError ? (
                      <p className="text-destructive text-xs">{closeError}</p>
                    ) : null}
                  </div>
                </>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

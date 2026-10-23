"use client";

import { Plus, Trash2 } from "lucide-react";
import {
  Controller,
  get,
  useFieldArray,
  useFormContext,
} from "react-hook-form";

import { EmptyState } from "@/components/common/empty-state";
import { AppDatePicker, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import type { BranchFormValues } from "@/features/branches/schemas/branch-form";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Dated overrides of the weekly schedule (public holidays, special days).
 * Dates use the shared `AppDatePicker` (`yyyy-MM-dd`).
 */
export function HolidayExceptionsEditor() {
  const { t } = useLocale();
  const {
    control,
    register,
    watch,
    formState: { errors },
  } = useFormContext<BranchFormValues>();
  const { fields, append, remove } = useFieldArray({
    control,
    name: "holiday_exceptions",
  });

  return (
    <div className="space-y-3">
      {fields.length === 0 ? (
        <EmptyState
          className="py-8"
          title={t("branches.holidays.empty_title")}
          description={t("branches.holidays.empty_description")}
        />
      ) : null}

      {fields.map((field, index) => {
        const base = `holiday_exceptions.${index}` as const;
        const closed = watch(`${base}.closed`);
        const openError = get(errors, `${base}.open`)?.message as
          string | undefined;
        const closeError = get(errors, `${base}.close`)?.message as
          string | undefined;
        return (
          <div
            key={field.id}
            className="border-border grid gap-3 rounded-lg border p-3 sm:grid-cols-[12rem_1fr_auto]"
          >
            <AppDatePicker
              name={`${base}.date`}
              label={t("branches.holidays.date")}
            />
            <div className="space-y-2">
              <Controller
                control={control}
                name={`${base}.closed`}
                render={({ field: closedField }) => (
                  <label className="flex items-center gap-2 pt-6 text-sm">
                    <Switch
                      checked={closedField.value}
                      onCheckedChange={closedField.onChange}
                    />
                    {t("branches.holidays.closed_all_day")}
                  </label>
                )}
              />
              {!closed ? (
                <div className="flex flex-wrap items-start gap-2">
                  <div className="space-y-1">
                    <Input
                      type="time"
                      step={60}
                      className={cn("w-32", openError && "border-destructive")}
                      aria-label={t("branches.hours.opens")}
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
                      {...register(`${base}.close`)}
                    />
                    {closeError ? (
                      <p className="text-destructive text-xs">{closeError}</p>
                    ) : null}
                  </div>
                </div>
              ) : null}
              <AppInput
                name={`${base}.note`}
                placeholder={t("branches.holidays.note_placeholder")}
                aria-label={t("branches.holidays.note")}
              />
            </div>
            <div className="flex items-start justify-end sm:pt-6">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="text-destructive hover:text-destructive size-8"
                aria-label={t("branches.holidays.remove")}
                onClick={() => remove(index)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          </div>
        );
      })}

      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() =>
          append({ date: "", closed: true, open: "", close: "", note: "" })
        }
      >
        <Plus className="size-4" />
        {t("branches.holidays.add")}
      </Button>
    </div>
  );
}

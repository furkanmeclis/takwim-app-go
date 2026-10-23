"use client";

import { ChevronDown, Coffee, Copy, Plus, Trash2 } from "lucide-react";
import { useState, type ReactNode } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { DAY_LABEL_KEYS } from "@/features/branches/constants";
import { WorkingHoursView } from "@/features/branches/components/working-hours-view";
import type { WeeklyHours } from "@/features/branches/types";
import {
  MAX_BREAKS_PER_SHIFT,
  MAX_SHIFTS_PER_DAY,
  SHIFT_DAY_KEYS,
} from "@/features/staff/constants";
import { useSetShifts } from "@/features/staff/hooks/use-staff";
import { cloneWeek, sortWeek } from "@/features/staff/lib/shifts";
import type {
  ShiftDayKey,
  StaffAssignment,
  WeeklyShifts,
} from "@/features/staff/types";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** API detail codes of the shift validation → i18n keys. */
const ERROR_KEYS: Record<string, string> = {
  overlap: "staff.shifts.errors.overlap",
  outside: "staff.shifts.errors.outside",
  branch_overlap: "staff.shifts.errors.branch_overlap",
  invalid: "staff.shifts.errors.invalid",
  too_many: "staff.shifts.errors.too_many",
};

type Errors = Record<string, string>;

/** `shifts.mon.0.breaks.1.start` → `mon.0.breaks.1` (row of the editor). */
function rowKey(field: string) {
  const path = field.replace(/^shifts\./, "").split(".");
  if (path[path.length - 1] === "start" || path[path.length - 1] === "end") {
    path.pop();
  }
  return path.join(".");
}

function validTime(value: string) {
  return /^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(value);
}

/** Client checks the API repeats: format and end after start. */
function localErrors(week: WeeklyShifts, message: string): Errors {
  const out: Errors = {};
  for (const day of SHIFT_DAY_KEYS) {
    week[day].forEach((shift, i) => {
      if (
        !validTime(shift.start) ||
        !validTime(shift.end) ||
        shift.end <= shift.start
      ) {
        out[`${day}.${i}`] = message;
      }
      shift.breaks.forEach((b, k) => {
        if (!validTime(b.start) || !validTime(b.end) || b.end <= b.start) {
          out[`${day}.${i}.breaks.${k}`] = message;
        }
      });
    });
  }
  return out;
}

function TimeRange({
  start,
  end,
  onChange,
  disabled,
  invalid,
  labels,
}: {
  start: string;
  end: string;
  onChange: (start: string, end: string) => void;
  disabled?: boolean;
  invalid?: boolean;
  labels: { start: string; end: string };
}) {
  return (
    <div className="flex items-center gap-2">
      <Input
        type="time"
        step={60}
        value={start}
        aria-label={labels.start}
        aria-invalid={invalid}
        disabled={disabled}
        className={cn("w-28", invalid && "border-destructive")}
        onChange={(event) => onChange(event.target.value, end)}
      />
      <span className="text-muted-foreground text-sm">–</span>
      <Input
        type="time"
        step={60}
        value={end}
        aria-label={labels.end}
        aria-invalid={invalid}
        disabled={disabled}
        className={cn("w-28", invalid && "border-destructive")}
        onChange={(event) => onChange(start, event.target.value)}
      />
    </div>
  );
}

/**
 * Weekly shifts of one person in one branch: several shifts a day, breaks
 * inside each shift; the branch's opening hours are shown for reference.
 */
export function ShiftEditorDialog({
  staffUuid,
  assignment,
  onOpenChange,
}: {
  staffUuid: string;
  assignment: StaffAssignment | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <AppDialogShell assignment={assignment} onOpenChange={onOpenChange}>
      {assignment ? (
        <ShiftEditor
          key={assignment.branch_uuid}
          staffUuid={staffUuid}
          assignment={assignment}
          onDone={() => onOpenChange(false)}
        />
      ) : null}
    </AppDialogShell>
  );
}

function AppDialogShell({
  assignment,
  onOpenChange,
  children,
}: {
  assignment: StaffAssignment | null;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <AppDialog
      open={Boolean(assignment)}
      onOpenChange={onOpenChange}
      title={t("staff.shifts.edit_title", {
        branch: assignment?.branch_name ?? "",
      })}
      description={t("staff.shifts.edit_description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
    >
      {children}
    </AppDialog>
  );
}

function ShiftEditor({
  staffUuid,
  assignment,
  onDone,
}: {
  staffUuid: string;
  assignment: StaffAssignment;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const save = useSetShifts(staffUuid);
  const [week, setWeek] = useState<WeeklyShifts>(() =>
    cloneWeek(assignment.shifts),
  );
  const [errors, setErrors] = useState<Errors>({});
  const [formError, setFormError] = useState<string | null>(null);
  const timeLabels = {
    start: t("staff.shifts.start"),
    end: t("staff.shifts.end"),
  };

  const update = (
    day: ShiftDayKey,
    fn: (shifts: WeeklyShifts[ShiftDayKey]) => void,
  ) => {
    setWeek((current) => {
      const next = cloneWeek(current);
      fn(next[day]);
      return next;
    });
    setErrors({});
    setFormError(null);
  };

  const copyToWeekdays = (day: ShiftDayKey) => {
    setWeek((current) => {
      const next = cloneWeek(current);
      for (const target of ["mon", "tue", "wed", "thu", "fri"] as const) {
        if (target !== day) next[target] = cloneWeek(current)[day];
      }
      return next;
    });
    setErrors({});
  };

  const submit = async () => {
    const sorted = sortWeek(week);
    setWeek(sorted);
    const local = localErrors(sorted, t("staff.shifts.errors.invalid"));
    if (Object.keys(local).length) {
      setErrors(local);
      return;
    }
    try {
      await save.mutateAsync({
        branchUuid: assignment.branch_uuid,
        shifts: sorted,
      });
      onDone();
    } catch (error) {
      if (error instanceof ApiError && error.details.length) {
        const next: Errors = {};
        for (const detail of error.details) {
          const key = ERROR_KEYS[detail.code ?? ""];
          next[rowKey(detail.field ?? "")] = key
            ? t(key)
            : (detail.message ?? t("common.error_generic"));
        }
        setErrors(next);
        setFormError(t("staff.shifts.errors.summary"));
      } else {
        setFormError(
          error instanceof Error ? error.message : t("common.error_generic"),
        );
      }
    }
  };

  return (
    <div className="space-y-4">
      <Collapsible className="rounded-lg border px-3 py-2 text-sm">
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="w-full justify-between px-0"
          >
            {t("staff.shifts.branch_hours")}
            <ChevronDown className="size-4" />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-2">
          <WorkingHoursView hours={assignment.branch_hours as WeeklyHours} />
        </CollapsibleContent>
      </Collapsible>

      <div className="divide-border border-border divide-y rounded-lg border">
        {SHIFT_DAY_KEYS.map((day) => {
          const shifts = week[day];
          return (
            <div key={day} className="space-y-2 px-3 py-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-sm font-medium">
                  {t(DAY_LABEL_KEYS[day])}
                  {shifts.length === 0 ? (
                    <span className="text-muted-foreground ml-2 font-normal">
                      {t("staff.shifts.day_off")}
                    </span>
                  ) : null}
                </span>
                <div className="flex gap-1">
                  {shifts.length > 0 ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => copyToWeekdays(day)}
                    >
                      <Copy className="size-4" />
                      {t("staff.shifts.copy_weekdays")}
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={shifts.length >= MAX_SHIFTS_PER_DAY}
                    onClick={() =>
                      update(day, (list) => {
                        const last = list[list.length - 1];
                        list.push(
                          last
                            ? { start: last.end, end: last.end, breaks: [] }
                            : { start: "09:00", end: "18:00", breaks: [] },
                        );
                      })
                    }
                  >
                    <Plus className="size-4" />
                    {t("staff.shifts.add_shift")}
                  </Button>
                </div>
              </div>
              {shifts.map((shift, i) => {
                const shiftError = errors[`${day}.${i}`];
                return (
                  <div key={i} className="bg-muted/40 space-y-2 rounded-md p-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <TimeRange
                        start={shift.start}
                        end={shift.end}
                        labels={timeLabels}
                        invalid={Boolean(shiftError)}
                        onChange={(start, end) =>
                          update(day, (list) => {
                            list[i] = { ...list[i]!, start, end };
                          })
                        }
                      />
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={shift.breaks.length >= MAX_BREAKS_PER_SHIFT}
                        onClick={() =>
                          update(day, (list) => {
                            list[i]!.breaks.push({
                              start: "12:00",
                              end: "13:00",
                            });
                          })
                        }
                      >
                        <Coffee className="size-4" />
                        {t("staff.shifts.add_break")}
                      </Button>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        aria-label={t("staff.shifts.remove_shift")}
                        onClick={() =>
                          update(day, (list) => {
                            list.splice(i, 1);
                          })
                        }
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                    {shiftError ? (
                      <p className="text-destructive text-xs">{shiftError}</p>
                    ) : null}
                    {shift.breaks.map((b, k) => {
                      const breakError = errors[`${day}.${i}.breaks.${k}`];
                      return (
                        <div key={k} className="space-y-1 pl-6">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-muted-foreground w-12 text-xs">
                              {t("staff.shifts.break")}
                            </span>
                            <TimeRange
                              start={b.start}
                              end={b.end}
                              labels={timeLabels}
                              invalid={Boolean(breakError)}
                              onChange={(start, end) =>
                                update(day, (list) => {
                                  list[i]!.breaks[k] = { start, end };
                                })
                              }
                            />
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              aria-label={t("staff.shifts.remove_break")}
                              onClick={() =>
                                update(day, (list) => {
                                  list[i]!.breaks.splice(k, 1);
                                })
                              }
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          </div>
                          {breakError ? (
                            <p className="text-destructive text-xs">
                              {breakError}
                            </p>
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>

      {formError ? (
        <p className="text-destructive text-sm">{formError}</p>
      ) : null}

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" onClick={onDone}>
          {t("form.cancel")}
        </Button>
        <Button
          type="button"
          disabled={save.isPending}
          onClick={() => void submit()}
        >
          {t("staff.actions.save")}
        </Button>
      </div>
    </div>
  );
}

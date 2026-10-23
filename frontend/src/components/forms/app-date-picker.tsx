"use client";

import { Controller, get, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { DatePicker } from "@/components/ui/date-picker";

type AppDatePickerProps = {
  name: string;
  label?: string;
  description?: string;
  placeholder?: string;
  className?: string;
  disabled?: boolean;
  /** Date and time (local `yyyy-MM-dd'T'HH:mm`). */
  withTime?: boolean;
};

/**
 * Form-bound date field. Value stored as `yyyy-MM-dd`, or local
 * `yyyy-MM-dd'T'HH:mm` with `withTime`.
 */
export function AppDatePicker({
  name,
  label,
  description,
  placeholder,
  className,
  disabled,
  withTime,
}: AppDatePickerProps) {
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = get(errors, name)?.message as string | undefined;

  return (
    <FormFieldShell
      name={name}
      label={label}
      description={description}
      error={error}
      className={className}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <DatePicker
            id={name}
            value={typeof field.value === "string" ? field.value : ""}
            onChange={field.onChange}
            placeholder={placeholder}
            disabled={disabled}
            withTime={withTime}
            aria-invalid={Boolean(error)}
            className={error ? "border-destructive" : undefined}
          />
        )}
      />
    </FormFieldShell>
  );
}

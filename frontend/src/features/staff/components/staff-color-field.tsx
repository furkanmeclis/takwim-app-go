"use client";

import { Check } from "lucide-react";
import { Controller, get, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { STAFF_COLORS } from "@/features/staff/constants";

/** Calendar color swatches bound to react-hook-form (`#RRGGBB`). */
export function StaffColorField({
  name,
  label,
  description,
  className,
}: {
  name: string;
  label?: string;
  description?: string;
  className?: string;
}) {
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
        render={({ field }) => {
          const value = String(field.value ?? "").toUpperCase();
          const colors = STAFF_COLORS.includes(value)
            ? STAFF_COLORS
            : [...STAFF_COLORS, value].filter(Boolean);
          return (
            <ToggleGroup
              type="single"
              value={value}
              onValueChange={(next) => {
                if (next) field.onChange(next);
              }}
              className="flex flex-wrap justify-start gap-2"
              aria-invalid={Boolean(error)}
            >
              {colors.map((color) => (
                <ToggleGroupItem
                  key={color}
                  value={color}
                  aria-label={color}
                  className="size-8 min-w-8 rounded-full p-0 data-[state=on]:ring-2 data-[state=on]:ring-offset-2"
                  style={{ backgroundColor: color }}
                >
                  {value === color ? (
                    <Check className="size-4 text-white" />
                  ) : null}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          );
        }}
      />
    </FormFieldShell>
  );
}

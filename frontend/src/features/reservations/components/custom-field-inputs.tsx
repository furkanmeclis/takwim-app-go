"use client";

import { AppCheckbox, AppInput, AppSelect } from "@/components/forms";
import type { FormQuestion } from "@/features/reservations/lib/custom-fields";
import { useLocale } from "@/providers/locale-provider";

/**
 * Inputs of the business's booking questions (TEC-905) under
 * `custom_fields.<key>`, so the API's field errors land on them.
 */
export function CustomFieldInputs({
  fields,
  disabled,
}: {
  fields: FormQuestion[];
  disabled?: boolean;
}) {
  const { t } = useLocale();
  return (
    <>
      {fields.map((field) => {
        const name = `custom_fields.${field.key}`;
        const label = field.required ? `${field.label} *` : field.label;
        const description = field.help || undefined;
        if (field.type === "checkbox") {
          return (
            <AppCheckbox
              key={field.key}
              name={name}
              label={label}
              description={description}
            />
          );
        }
        if (field.type === "select") {
          return (
            <AppSelect
              key={field.key}
              name={name}
              label={label}
              description={description}
              placeholder={t("reservations.custom_fields.choose")}
              options={field.options.map((option) => ({
                value: option,
                label: option,
              }))}
              disabled={disabled}
            />
          );
        }
        return (
          <AppInput
            key={field.key}
            name={name}
            label={label}
            description={description}
            type={field.type === "number" ? "number" : "text"}
            inputMode={field.type === "number" ? "decimal" : undefined}
            maxLength={field.type === "text" ? 500 : undefined}
            disabled={disabled}
          />
        );
      })}
    </>
  );
}

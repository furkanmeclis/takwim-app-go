"use client";

import { useMemo } from "react";
import { Controller, get, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxChipsInput,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxItem,
  ComboboxList,
  ComboboxValue,
  useComboboxAnchor,
} from "@/components/ui/combobox";

export type MultiComboboxOption = {
  value: string;
  label: string;
  /** Muted second line in the list (e.g. a code or a hint). */
  hint?: string;
};

type MultiComboboxProps = {
  id?: string;
  options: MultiComboboxOption[];
  value: string[];
  onValueChange: (next: string[]) => void;
  placeholder?: string;
  emptyText?: string;
  disabled?: boolean;
  invalid?: boolean;
};

/**
 * shadcn multi-select combobox (chips): type to filter, pick several values,
 * remove a chip with its ✕. Values are strings; labels come from `options`.
 */
export function MultiCombobox({
  id,
  options,
  value,
  onValueChange,
  placeholder,
  emptyText,
  disabled,
  invalid,
}: MultiComboboxProps) {
  const anchor = useComboboxAnchor();
  const byValue = useMemo(
    () => new Map(options.map((option) => [option.value, option])),
    [options],
  );
  const items = useMemo(() => options.map((option) => option.value), [options]);
  const labelOf = (v: string) => byValue.get(v)?.label ?? v;

  return (
    <Combobox
      multiple
      autoHighlight
      items={items}
      value={value}
      onValueChange={(next) => onValueChange((next as string[]) ?? [])}
      itemToStringLabel={labelOf}
      disabled={disabled}
    >
      <ComboboxChips ref={anchor} className="w-full">
        <ComboboxValue>
          {(selected: string[]) => (
            <>
              {selected.map((v) => (
                <ComboboxChip key={v}>{labelOf(v)}</ComboboxChip>
              ))}
              <ComboboxChipsInput
                id={id}
                placeholder={selected.length ? undefined : placeholder}
                aria-invalid={invalid || undefined}
                disabled={disabled}
              />
            </>
          )}
        </ComboboxValue>
      </ComboboxChips>
      <ComboboxContent anchor={anchor}>
        <ComboboxEmpty>{emptyText}</ComboboxEmpty>
        <ComboboxList>
          {(item: string) => {
            const option = byValue.get(item);
            return (
              <ComboboxItem key={item} value={item}>
                <span className="flex min-w-0 flex-col">
                  <span className="truncate">{option?.label ?? item}</span>
                  {option?.hint ? (
                    <span className="text-muted-foreground truncate text-xs">
                      {option.hint}
                    </span>
                  ) : null}
                </span>
              </ComboboxItem>
            );
          }}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  );
}

type AppMultiComboboxProps = {
  name: string;
  label?: string;
  description?: string;
  className?: string;
} & Omit<MultiComboboxProps, "id" | "value" | "onValueChange" | "invalid">;

/** RHF field around `MultiCombobox` (value: `string[]`). */
export function AppMultiCombobox({
  name,
  label,
  description,
  className,
  ...props
}: AppMultiComboboxProps) {
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
          <MultiCombobox
            id={name}
            value={(field.value as string[] | undefined) ?? []}
            onValueChange={field.onChange}
            invalid={Boolean(error)}
            {...props}
          />
        )}
      />
    </FormFieldShell>
  );
}

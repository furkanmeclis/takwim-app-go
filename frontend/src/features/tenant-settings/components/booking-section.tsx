"use client";

import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";

import { EmptyState } from "@/components/common/empty-state";
import {
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormSection,
} from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SECTOR_STEP_LABEL_KEYS } from "@/features/sectors/constants";
import { useSector } from "@/features/sectors/hooks/use-sector";
import {
  BOOKING_FIELD_TYPES,
  BOOKING_FIELD_TYPE_KEYS,
  MAX_BOOKING_FIELDS,
} from "@/features/tenant-settings/constants";
import type { SettingsFormValues } from "@/features/tenant-settings/schemas/settings-form";
import type { BookingStep } from "@/features/tenant-settings/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type SectionProps = { disabled?: boolean };

/** Optional steps of a sector flow and the form switch that shows them. */
const OPTIONAL_STEPS: {
  step: BookingStep;
  name: "booking.show_staff" | "booking.show_products" | "booking.show_area";
  hintKey: string;
}[] = [
  {
    step: "staff",
    name: "booking.show_staff",
    hintKey: "tenant_settings.booking.show_staff_hint",
  },
  {
    step: "products",
    name: "booking.show_products",
    hintKey: "tenant_settings.booking.show_products_hint",
  },
  {
    step: "area",
    name: "booking.show_area",
    hintKey: "tenant_settings.booking.show_area_hint",
  },
];

/**
 * Akış ve form (TEC-905): the optional steps of the sector's booking flow
 * the customer screens show, and the business's own booking questions.
 * Required steps and every capacity / availability check stay; the API
 * enforces the same choices for the booking page, widget and mobile app.
 */
export function BookingSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  const { sector } = useSector();
  const flow = sector?.flow ?? [];
  const optional = OPTIONAL_STEPS.filter((o) => flow.includes(o.step));
  const values = useWatch<SettingsFormValues, "booking">({ name: "booking" });
  const hidden = new Set(
    OPTIONAL_STEPS.filter((o) => values && !values[nameKey(o.name)]).map(
      (o) => o.step,
    ),
  );

  return (
    <div className="space-y-4">
      <FormSection
        title={t("tenant_settings.booking.flow_title")}
        description={t("tenant_settings.booking.flow_desc")}
      >
        <div className="flex flex-wrap items-center gap-1.5">
          {flow.map((step, index) => (
            <span key={step} className="flex items-center gap-1.5">
              {index > 0 ? (
                <span className="text-muted-foreground text-xs">→</span>
              ) : null}
              <Badge
                variant={
                  hidden.has(step as BookingStep) ? "outline" : "secondary"
                }
                className={cn(
                  hidden.has(step as BookingStep) &&
                    "text-muted-foreground line-through",
                )}
              >
                {t(SECTOR_STEP_LABEL_KEYS[step])}
              </Badge>
            </span>
          ))}
        </div>
        {optional.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("tenant_settings.booking.no_optional_steps")}
          </p>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {optional.map((o) => (
              <AppSwitch
                key={o.step}
                name={o.name}
                label={t("tenant_settings.booking.show_step", {
                  step: t(SECTOR_STEP_LABEL_KEYS[o.step]),
                })}
                description={t(o.hintKey)}
                disabled={disabled}
              />
            ))}
          </div>
        )}
        <p className="text-muted-foreground text-xs">
          {t("tenant_settings.booking.locked_note")}
        </p>
      </FormSection>
      <FormSection
        title={t("tenant_settings.booking.fields_title")}
        description={t("tenant_settings.booking.fields_desc")}
      >
        <BookingFieldsEditor disabled={disabled} />
      </FormSection>
    </div>
  );
}

function nameKey(name: (typeof OPTIONAL_STEPS)[number]["name"]) {
  return name.slice("booking.".length) as
    "show_staff" | "show_products" | "show_area";
}

/** The booking questions: label, type, choices, where they are asked. */
function BookingFieldsEditor({ disabled }: SectionProps) {
  const { t } = useLocale();
  const { control } = useFormContext<SettingsFormValues>();
  const { fields, append, remove, move } = useFieldArray({
    control,
    name: "booking.fields",
  });
  const types = useWatch<SettingsFormValues, "booking.fields">({
    name: "booking.fields",
  });

  return (
    <div className="space-y-3">
      {fields.length === 0 ? (
        <EmptyState
          className="py-8"
          title={t("tenant_settings.booking.fields_empty_title")}
          description={t("tenant_settings.booking.fields_empty_description")}
        />
      ) : null}
      {fields.map((field, index) => {
        const base = `booking.fields.${index}` as const;
        const type = types?.[index]?.type;
        return (
          <div
            key={field.id}
            className="border-border space-y-3 rounded-lg border p-3"
          >
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">
                {t("tenant_settings.booking.field_number", {
                  number: index + 1,
                })}
              </p>
              <div className="flex gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label={t("tenant_settings.booking.move_up")}
                  disabled={disabled || index === 0}
                  onClick={() => move(index, index - 1)}
                >
                  <ArrowUp className="size-4" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label={t("tenant_settings.booking.move_down")}
                  disabled={disabled || index === fields.length - 1}
                  onClick={() => move(index, index + 1)}
                >
                  <ArrowDown className="size-4" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="text-destructive hover:text-destructive size-8"
                  aria-label={t("tenant_settings.booking.remove_field")}
                  disabled={disabled}
                  onClick={() => remove(index)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <AppInput
                name={`${base}.label`}
                label={t("tenant_settings.booking.field_label")}
                maxLength={80}
                disabled={disabled}
              />
              <AppSelect
                name={`${base}.type`}
                label={t("tenant_settings.booking.field_type")}
                options={BOOKING_FIELD_TYPES.map((value) => ({
                  value,
                  label: t(BOOKING_FIELD_TYPE_KEYS[value]),
                }))}
                disabled={disabled}
              />
              {type === "select" ? (
                <AppTextarea
                  name={`${base}.options`}
                  label={t("tenant_settings.booking.field_options")}
                  description={t("tenant_settings.booking.field_options_hint")}
                  rows={3}
                  className="sm:col-span-2"
                  disabled={disabled}
                />
              ) : null}
              <AppInput
                name={`${base}.help`}
                label={t("tenant_settings.booking.field_help")}
                maxLength={200}
                className="sm:col-span-2"
                disabled={disabled}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <AppSwitch
                name={`${base}.required`}
                label={t("tenant_settings.booking.field_required")}
                description={
                  type === "checkbox"
                    ? t("tenant_settings.booking.field_required_checkbox")
                    : undefined
                }
                disabled={disabled}
              />
              <AppSwitch
                name={`${base}.online`}
                label={t("tenant_settings.booking.field_online")}
                disabled={disabled}
              />
              <AppSwitch
                name={`${base}.panel`}
                label={t("tenant_settings.booking.field_panel")}
                disabled={disabled}
              />
            </div>
          </div>
        );
      })}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled || fields.length >= MAX_BOOKING_FIELDS}
        onClick={() =>
          append({
            key: "",
            label: "",
            type: "text",
            options: "",
            required: false,
            online: true,
            panel: true,
            help: "",
          })
        }
      >
        <Plus className="size-4" />
        {t("tenant_settings.booking.add_field")}
      </Button>
    </div>
  );
}

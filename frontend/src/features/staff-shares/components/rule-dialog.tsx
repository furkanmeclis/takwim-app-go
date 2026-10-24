"use client";

import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { normalizeAmount, useBusinessToday } from "@/features/cash/lib/format";
import { useServicesList } from "@/features/services/hooks/use-services";
import {
  BASIS_LABEL_KEYS,
  ITEM_TYPE_LABEL_KEYS,
  MAX_NOTE,
  STAFF_SHARE_BASES,
  STAFF_SHARE_ITEM_TYPES,
} from "@/features/staff-shares/constants";
import { useSaveStaffShareRule } from "@/features/staff-shares/hooks/use-staff-shares";
import type {
  StaffShareRule,
  StaffShareRuleInput,
} from "@/features/staff-shares/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "staff-share-rule-form";
const AMOUNT = /^\d+([.,]\d{1,2})?$/;

type Translate = (key: string) => string;

function createRuleSchema(t: Translate) {
  return z
    .object({
      item_type: z.enum(STAFF_SHARE_ITEM_TYPES as ["service", "product"]),
      service_uuid: z.string(),
      basis: z.enum(STAFF_SHARE_BASES as ["percent", "fixed"]),
      value: z
        .string()
        .trim()
        .refine((v) => AMOUNT.test(v), t("staff_shares.validation.amount")),
      valid_from: z.string().min(1, t("staff_shares.validation.date_required")),
      note: z.string().max(MAX_NOTE, t("staff_shares.validation.note_max")),
    })
    .superRefine((values, ctx) => {
      const n = Number(values.value.replace(",", "."));
      if (values.basis === "percent" && n > 100) {
        ctx.addIssue({
          code: "custom",
          path: ["value"],
          message: t("staff_shares.validation.rate_max"),
        });
      }
    });
}

type RuleFormValues = z.infer<ReturnType<typeof createRuleSchema>>;

function RuleFields({ editing }: { editing: boolean }) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const { control } = useFormContext<RuleFormValues>();
  const itemType = useWatch({ control, name: "item_type" });
  const basis = useWatch({ control, name: "basis" });
  const services = useServicesList({
    limit: 100,
    offset: 0,
    sort: "name",
    is_active: "true",
  });
  const serviceOptions = useMemo(
    () =>
      (services.data?.items ?? []).map((service) => ({
        value: service.uuid,
        label: service.name,
      })),
    [services.data?.items],
  );

  return (
    <>
      {!editing ? (
        <AppToggleGroup
          name="item_type"
          label={t("staff_shares.rules.fields.item_type")}
          className="sm:col-span-2"
          options={STAFF_SHARE_ITEM_TYPES.map((value) => ({
            value,
            label: t(ITEM_TYPE_LABEL_KEYS[value]),
          }))}
        />
      ) : null}
      {!editing && itemType === "service" ? (
        <AppCombobox
          name="service_uuid"
          label={t("staff_shares.rules.fields.service")}
          description={t("staff_shares.rules.fields.service_hint")}
          placeholder={t("staff_shares.rules.fields.service_placeholder")}
          searchPlaceholder={t("staff_shares.rules.fields.service_search")}
          emptyText={t("staff_shares.rules.fields.service_empty")}
          options={serviceOptions}
          className="sm:col-span-2"
        />
      ) : null}
      <AppToggleGroup
        name="basis"
        label={t("staff_shares.rules.fields.basis")}
        className="sm:col-span-2"
        options={STAFF_SHARE_BASES.map((value) => ({
          value,
          label: t(BASIS_LABEL_KEYS[value]),
        }))}
      />
      <AppInput
        name="value"
        inputMode="decimal"
        label={
          basis === "percent"
            ? t("staff_shares.rules.fields.rate")
            : t("staff_shares.rules.fields.fixed_amount", {
                currency: organization?.currency ?? "TRY",
              })
        }
        description={
          basis === "percent"
            ? t("staff_shares.rules.fields.rate_hint")
            : t("staff_shares.rules.fields.fixed_amount_hint")
        }
      />
      <AppDatePicker
        name="valid_from"
        label={t("staff_shares.rules.fields.valid_from")}
        description={t("staff_shares.rules.fields.valid_from_hint")}
      />
      <AppTextarea
        name="note"
        label={t("staff_shares.rules.fields.note")}
        className="sm:col-span-2"
      />
    </>
  );
}

function toRequest(
  staffUuid: string,
  values: RuleFormValues,
  editing: boolean,
): StaffShareRuleInput {
  const value = normalizeAmount(values.value);
  const body: StaffShareRuleInput = {
    basis: values.basis,
    valid_from: values.valid_from,
    note: values.note.trim(),
    ...(values.basis === "percent" ? { rate: value } : { fixed_amount: value }),
  };
  if (editing) return body;
  return {
    ...body,
    staff_uuid: staffUuid,
    item_type: values.item_type,
    service_uuid:
      values.item_type === "service" && values.service_uuid
        ? values.service_uuid
        : undefined,
  };
}

/**
 * Create or change a share rule of a staff member: the default of
 * services or products, or one service's own rate, from `valid_from` on.
 * Shares already calculated keep the rate they were calculated with.
 */
export function RuleDialog({
  staffUuid,
  rule,
  open,
  onOpenChange,
}: {
  staffUuid: string;
  /** Rule to edit; `null` creates a new one. */
  rule: StaffShareRule | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const save = useSaveStaffShareRule();
  const schema = useMemo(() => createRuleSchema(t), [t]);
  const editing = Boolean(rule);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        editing
          ? t("staff_shares.rules.edit_title")
          : t("staff_shares.rules.create_title")
      }
      description={t("staff_shares.rules.dialog_description")}
      className="sm:max-w-xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {editing
              ? t("staff_shares.actions.save")
              : t("staff_shares.rules.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<RuleFormValues>
          key={rule?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            item_type: rule?.item_type ?? "service",
            service_uuid: rule?.service?.uuid ?? "",
            basis: rule?.basis ?? "percent",
            value:
              (rule?.basis === "fixed" ? rule.fixed_amount : rule?.rate) ?? "",
            valid_from: rule?.valid_from ?? today,
            note: rule?.note ?? "",
          }}
          onSubmit={async (values) => {
            await save.mutateAsync({
              uuid: rule?.uuid,
              body: toRequest(staffUuid, values, editing),
            });
            onOpenChange(false);
          }}
        >
          <RuleFields editing={editing} />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

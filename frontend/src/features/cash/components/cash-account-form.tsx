"use client";

import { Landmark, Wallet } from "lucide-react";
import { useMemo } from "react";

import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { CASH_ACCOUNT_KINDS, HEAD_OFFICE } from "@/features/cash/constants";
import { kindLabel } from "@/features/cash/lib/format";
import {
  createAccountFormSchema,
  type AccountFormValues,
} from "@/features/cash/schemas/cash-forms";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type CashAccountFormProps = {
  mode: "create" | "edit";
  defaultValues: AccountFormValues;
  /** Opening balance / date are fixed once the account has movements. */
  openingLocked?: boolean;
  /** Members limited to some branches cannot open business-wide accounts. */
  allowHeadOffice: boolean;
  isSubmitting?: boolean;
  onSubmit: (values: AccountFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Create / edit form: identity (branch, kind, name), opening, details. */
export function CashAccountForm({
  mode,
  defaultValues,
  openingLocked,
  allowHeadOffice,
  isSubmitting,
  onSubmit,
  onCancel,
}: CashAccountFormProps) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const schema = useMemo(() => createAccountFormSchema(t), [t]);
  const branches = useAccessibleBranches();
  const currency = organization?.currency ?? "TRY";

  const branchOptions = useMemo(() => {
    const items = (branches.data?.items ?? []).map((branch) => ({
      value: branch.uuid,
      label: `${branch.name} (${branch.code})`,
    }));
    return allowHeadOffice || defaultValues.branch_uuid === HEAD_OFFICE
      ? [{ value: HEAD_OFFICE, label: t("cash.head_office") }, ...items]
      : items;
  }, [allowHeadOffice, branches.data?.items, defaultValues.branch_uuid, t]);

  const kindOptions = useMemo(
    () =>
      CASH_ACCOUNT_KINDS.map((kind) => ({
        value: kind,
        label: kindLabel(t, kind),
      })),
    [t],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("cash.form.section_identity"),
          description: t("cash.form.section_identity_desc"),
          icon: Wallet,
        },
        {
          key: "details",
          label: t("cash.form.section_details"),
          description: t("cash.form.section_details_desc"),
          icon: Landmark,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => {
        const kind = form.watch("kind");
        return (
          <FormLayout navItems={sections.navItems}>
            <FormSection
              id={sections.id("identity")}
              title={t("cash.form.section_identity")}
              description={t("cash.form.section_identity_desc")}
              columns={2}
            >
              <AppSelect
                name="branch_uuid"
                label={t("cash.fields.branch")}
                description={
                  mode === "edit"
                    ? t("cash.fields.fixed_hint")
                    : t("cash.fields.branch_hint")
                }
                placeholder={t("cash.fields.branch_placeholder")}
                options={branchOptions}
                disabled={mode === "edit"}
              />
              <AppSelect
                name="kind"
                label={t("cash.fields.kind")}
                description={
                  mode === "edit"
                    ? t("cash.fields.fixed_hint")
                    : t("cash.fields.kind_hint")
                }
                options={kindOptions}
                disabled={mode === "edit"}
              />
              <AppInput
                name="name"
                label={t("cash.fields.name")}
                description={t("cash.fields.name_hint")}
              />
              <AppInput
                name="sort_order"
                type="number"
                min={0}
                label={t("cash.fields.sort_order")}
                description={t("cash.fields.sort_order_hint")}
              />
              <AppInput
                name="opening_balance"
                inputMode="decimal"
                label={t("cash.fields.opening_balance", { currency })}
                description={
                  openingLocked
                    ? t("cash.fields.opening_locked")
                    : t("cash.fields.opening_balance_hint")
                }
                disabled={openingLocked}
              />
              <AppDatePicker
                name="opening_date"
                label={t("cash.fields.opening_date")}
                description={
                  openingLocked ? t("cash.fields.opening_locked") : undefined
                }
                disabled={openingLocked}
              />
            </FormSection>

            <FormSection
              id={sections.id("details")}
              title={t("cash.form.section_details")}
              description={t("cash.form.section_details_desc")}
              columns={2}
            >
              {kind === "bank" ? (
                <>
                  <AppInput
                    name="bank_name"
                    label={t("cash.fields.bank_name")}
                  />
                  <AppInput
                    name="iban"
                    label={t("cash.fields.iban")}
                    description={t("cash.fields.iban_hint")}
                  />
                </>
              ) : null}
              {kind === "online" ? (
                <AppInput
                  name="provider"
                  label={t("cash.fields.provider")}
                  description={t("cash.fields.provider_hint")}
                />
              ) : null}
              <AppTextarea
                name="note"
                label={t("cash.fields.note")}
                className="sm:col-span-2"
              />
            </FormSection>

            <FormActions>
              <Button type="button" variant="outline" onClick={onCancel}>
                {t("form.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={isSubmitting || form.formState.isSubmitting}
              >
                {mode === "create"
                  ? t("cash.actions.create")
                  : t("cash.actions.save")}
              </Button>
            </FormActions>
          </FormLayout>
        );
      }}
    </AppForm>
  );
}

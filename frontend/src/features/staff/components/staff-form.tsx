"use client";

import { Contact, Store, UserRound } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { StaffColorField } from "@/features/staff/components/staff-color-field";
import {
  useAccessibleBranches,
  useLinkableMembers,
} from "@/features/staff/hooks/use-staff";
import {
  NO_ACCOUNT,
  createStaffFormSchema,
  type StaffFormValues,
} from "@/features/staff/schemas/staff-form";
import type { StaffAccount } from "@/features/staff/types";
import { useLocale } from "@/providers/locale-provider";

type StaffFormProps = {
  mode: "create" | "edit";
  defaultValues: StaffFormValues;
  /** Account linked today (edit), offered next to the free members. */
  currentAccount?: StaffAccount | null;
  isSubmitting?: boolean;
  onSubmit: (values: StaffFormValues) => Promise<void>;
  onCancel: () => void;
};

function accountLabel(account: StaffAccount) {
  const name = `${account.name} ${account.surname}`.trim();
  return name ? `${name} · ${account.email}` : account.email;
}

/** Create / edit form: profile, contact & panel account, branches. */
export function StaffForm({
  mode,
  defaultValues,
  currentAccount,
  isSubmitting,
  onSubmit,
  onCancel,
}: StaffFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createStaffFormSchema(t), [t]);
  const members = useLinkableMembers();
  const branches = useAccessibleBranches();

  const accountOptions = useMemo(() => {
    const list = [...(members.data?.items ?? [])];
    if (currentAccount && !list.some((m) => m.uuid === currentAccount.uuid)) {
      list.unshift(currentAccount);
    }
    return [
      { value: NO_ACCOUNT, label: t("staff.account.none") },
      ...list.map((member) => ({
        value: member.uuid,
        label: accountLabel(member),
      })),
    ];
  }, [currentAccount, members.data?.items, t]);

  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: branch.name,
        hint: branch.code,
      })),
    [branches.data?.items],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "profile",
          label: t("staff.form.section_profile"),
          description: t("staff.form.section_profile_desc"),
          icon: UserRound,
        },
        {
          key: "contact",
          label: t("staff.form.section_contact"),
          description: t("staff.form.section_contact_desc"),
          icon: Contact,
        },
        {
          key: "branches",
          label: t("staff.form.section_branches"),
          description: t("staff.form.section_branches_desc"),
          icon: Store,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("profile")}
            title={t("staff.form.section_profile")}
            description={t("staff.form.section_profile_desc")}
            columns={2}
          >
            <AppInput name="name" label={t("staff.fields.name")} />
            <AppInput
              name="title"
              label={t("staff.fields.title")}
              description={t("staff.fields.title_hint")}
            />
            <StaffColorField
              name="color"
              label={t("staff.fields.color")}
              description={t("staff.fields.color_hint")}
              className="sm:col-span-2"
            />
            <AppSwitch
              name="show_online"
              label={t("staff.fields.show_online")}
              description={t("staff.fields.show_online_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("staff.fields.is_active")}
              description={t("staff.fields.is_active_hint")}
            />
            <AppTextarea
              name="bio"
              label={t("staff.fields.bio")}
              description={t("staff.fields.bio_hint")}
              className="sm:col-span-2"
            />
          </FormSection>

          <FormSection
            id={sections.id("contact")}
            title={t("staff.form.section_contact")}
            description={t("staff.form.section_contact_desc")}
            columns={2}
          >
            <AppInput name="phone" type="tel" label={t("staff.fields.phone")} />
            <AppInput
              name="email"
              type="email"
              label={t("staff.fields.email")}
            />
            <AppSelect
              name="account_uuid"
              label={t("staff.fields.account")}
              description={t("staff.fields.account_hint")}
              options={accountOptions}
              className="sm:col-span-2"
            />
          </FormSection>

          <FormSection
            id={sections.id("branches")}
            title={t("staff.form.section_branches")}
            description={t("staff.form.section_branches_desc")}
          >
            <AppMultiCombobox
              name="branch_uuids"
              label={t("staff.fields.branches")}
              description={t("staff.fields.branches_hint")}
              options={branchOptions}
              placeholder={t("staff.fields.branches_placeholder")}
              emptyText={t("staff.fields.branches_empty")}
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
                ? t("staff.actions.create")
                : t("staff.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}

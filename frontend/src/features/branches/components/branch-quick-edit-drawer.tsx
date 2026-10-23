"use client";

import Link from "next/link";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useUpdateBranch } from "@/features/branches/hooks/use-branches";
import {
  createQuickEditSchema,
  type QuickEditValues,
} from "@/features/branches/schemas/branch-form";
import type { Branch } from "@/features/branches/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "branch-quick-edit";

/** Drawer for the fields people change most, without leaving the list. */
export function BranchQuickEditDrawer({
  branch,
  onOpenChange,
}: {
  branch: Branch | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateBranch();
  const schema = useMemo(() => createQuickEditSchema(t), [t]);

  return (
    <EntityDrawer
      open={Boolean(branch)}
      onOpenChange={onOpenChange}
      title={t("branches.quick_edit.title")}
      description={
        branch
          ? t("branches.quick_edit.description", {
              code: branch.code,
              name: branch.name,
            })
          : undefined
      }
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {branch ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.branches.edit(slug, branch.uuid)}>
                {t("branches.quick_edit.full_form")}
              </Link>
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={update.isPending}>
              {t("branches.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {branch ? (
        <AppForm<QuickEditValues>
          key={branch.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: branch.name,
            phone: branch.phone,
            email: branch.email,
            prep_minutes: String(branch.prep_minutes),
            is_active: branch.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: branch.uuid,
              body: {
                name: values.name.trim(),
                phone: values.phone.trim(),
                email: values.email.trim(),
                prep_minutes: Number(values.prep_minutes),
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("branches.fields.name")} />
          <AppInput
            name="phone"
            type="tel"
            label={t("branches.fields.phone")}
          />
          <AppInput
            name="email"
            type="email"
            label={t("branches.fields.email")}
          />
          <AppInput
            name="prep_minutes"
            type="number"
            min={0}
            label={t("branches.fields.prep_minutes")}
          />
          <AppSwitch name="is_active" label={t("branches.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

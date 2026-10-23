"use client";

import Link from "next/link";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useUpdateStaff } from "@/features/staff/hooks/use-staff";
import {
  createQuickEditSchema,
  type QuickEditValues,
} from "@/features/staff/schemas/staff-form";
import type { Staff } from "@/features/staff/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "staff-quick-edit";

/** Drawer for the fields people change most, without leaving the list. */
export function StaffQuickEditDrawer({
  staff,
  onOpenChange,
}: {
  staff: Staff | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateStaff();
  const schema = useMemo(() => createQuickEditSchema(t), [t]);

  return (
    <EntityDrawer
      open={Boolean(staff)}
      onOpenChange={onOpenChange}
      title={t("staff.quick_edit.title")}
      description={staff?.name}
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {staff ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.staff.edit(slug, staff.uuid)}>
                {t("staff.quick_edit.full_form")}
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
              {t("staff.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {staff ? (
        <AppForm<QuickEditValues>
          key={staff.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: staff.name,
            title: staff.title,
            phone: staff.phone,
            email: staff.email,
            show_online: staff.show_online,
            is_active: staff.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: staff.uuid,
              body: {
                name: values.name.trim(),
                title: values.title.trim(),
                phone: values.phone.trim(),
                email: values.email.trim(),
                show_online: values.show_online,
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("staff.fields.name")} />
          <AppInput name="title" label={t("staff.fields.title")} />
          <AppInput name="phone" type="tel" label={t("staff.fields.phone")} />
          <AppInput name="email" type="email" label={t("staff.fields.email")} />
          <AppSwitch name="show_online" label={t("staff.fields.show_online")} />
          <AppSwitch name="is_active" label={t("staff.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

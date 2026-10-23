"use client";

import Link from "next/link";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { MAX_CAPACITY } from "@/features/resources/constants";
import { useUpdateResource } from "@/features/resources/hooks/use-resources";
import {
  createQuickEditSchema,
  type QuickEditValues,
} from "@/features/resources/schemas/resource-form";
import type { Resource } from "@/features/resources/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "resource-quick-edit";

/** Drawer for the fields people change most, without leaving the list. */
export function ResourceQuickEditDrawer({
  resource,
  onOpenChange,
}: {
  resource: Resource | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateResource();
  const schema = useMemo(() => createQuickEditSchema(t), [t]);

  return (
    <EntityDrawer
      open={Boolean(resource)}
      onOpenChange={onOpenChange}
      title={t("resources.quick_edit.title")}
      description={resource?.name}
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {resource ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.resources.edit(slug, resource.uuid)}>
                {t("resources.quick_edit.full_form")}
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
              {t("resources.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {resource ? (
        <AppForm<QuickEditValues>
          key={resource.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: resource.name,
            zone: resource.zone,
            min_capacity: String(resource.min_capacity),
            capacity: String(resource.capacity),
            show_online: resource.show_online,
            is_active: resource.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: resource.uuid,
              body: {
                name: values.name.trim(),
                zone: values.zone.trim(),
                min_capacity: Number(values.min_capacity),
                capacity: Number(values.capacity),
                show_online: values.show_online,
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("resources.fields.name")} />
          <AppInput name="zone" label={t("resources.fields.zone")} />
          <div className="grid grid-cols-2 gap-4">
            <AppInput
              name="min_capacity"
              type="number"
              min={1}
              max={MAX_CAPACITY}
              label={t("resources.fields.min_capacity")}
            />
            <AppInput
              name="capacity"
              type="number"
              min={1}
              max={MAX_CAPACITY}
              label={t("resources.fields.capacity")}
            />
          </div>
          <AppSwitch
            name="show_online"
            label={t("resources.fields.show_online")}
          />
          <AppSwitch name="is_active" label={t("resources.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

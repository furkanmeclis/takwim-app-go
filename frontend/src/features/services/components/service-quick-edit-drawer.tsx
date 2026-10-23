"use client";

import Link from "next/link";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { MAX_DURATION, MIN_DURATION } from "@/features/services/constants";
import { useUpdateService } from "@/features/services/hooks/use-services";
import {
  createQuickEditSchema,
  normalizePrice,
  type QuickEditValues,
} from "@/features/services/schemas/service-form";
import type { Service } from "@/features/services/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "service-quick-edit";

/** Drawer for the fields people change most, without leaving the list. */
export function ServiceQuickEditDrawer({
  service,
  onOpenChange,
}: {
  service: Service | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateService();
  const schema = useMemo(() => createQuickEditSchema(t), [t]);

  return (
    <EntityDrawer
      open={Boolean(service)}
      onOpenChange={onOpenChange}
      title={t("services.quick_edit.title")}
      description={service?.name}
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {service ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.services.edit(slug, service.uuid)}>
                {t("services.quick_edit.full_form")}
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
              {t("services.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {service ? (
        <AppForm<QuickEditValues>
          key={service.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: service.name,
            duration_minutes: String(service.duration_minutes),
            price: service.price,
            show_online: service.show_online,
            is_active: service.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: service.uuid,
              body: {
                name: values.name.trim(),
                duration_minutes: Number(values.duration_minutes),
                price: normalizePrice(values.price) ?? "0.00",
                show_online: values.show_online,
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("services.fields.name")} />
          <div className="grid grid-cols-2 gap-4">
            <AppInput
              name="duration_minutes"
              type="number"
              min={MIN_DURATION}
              max={MAX_DURATION}
              label={t("services.fields.duration")}
            />
            <AppInput
              name="price"
              inputMode="decimal"
              label={t("services.fields.price_short")}
            />
          </div>
          <AppSwitch
            name="show_online"
            label={t("services.fields.show_online")}
          />
          <AppSwitch name="is_active" label={t("services.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

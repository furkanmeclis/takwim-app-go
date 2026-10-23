"use client";

import Link from "next/link";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useUpdateCustomer } from "@/features/customers/hooks/use-customers";
import {
  createQuickEditSchema,
  splitTags,
  type QuickEditValues,
} from "@/features/customers/schemas/customer-form";
import type { Customer } from "@/features/customers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-quick-edit";

/** Drawer for name, phone and tags, without leaving the list. */
export function CustomerQuickEditDrawer({
  customer,
  onOpenChange,
}: {
  customer: Customer | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateCustomer();
  const schema = useMemo(() => createQuickEditSchema(t), [t]);

  return (
    <EntityDrawer
      open={Boolean(customer)}
      onOpenChange={onOpenChange}
      title={t("customers.quick_edit.title")}
      description={customer?.full_name}
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {customer ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.customers.edit(slug, customer.uuid)}>
                {t("customers.quick_edit.full_form")}
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
              {t("customers.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {customer ? (
        <AppForm<QuickEditValues>
          key={customer.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            first_name: customer.first_name,
            last_name: customer.last_name,
            phone: customer.phone_display,
            tags: customer.tags.join(", "),
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: customer.uuid,
              body: {
                first_name: values.first_name.trim(),
                last_name: values.last_name.trim(),
                phone: values.phone.trim() || null,
                tags: splitTags(values.tags),
              },
            });
            onOpenChange(false);
          }}
        >
          <div className="grid grid-cols-2 gap-4">
            <AppInput
              name="first_name"
              label={t("customers.fields.first_name")}
            />
            <AppInput
              name="last_name"
              label={t("customers.fields.last_name")}
            />
          </div>
          <AppInput
            name="phone"
            type="tel"
            inputMode="tel"
            label={t("customers.fields.phone")}
            description={
              customer.verification
                ? t("customers.fields.phone_hint_verified")
                : undefined
            }
          />
          <AppInput
            name="tags"
            label={t("customers.fields.tags")}
            description={t("customers.fields.tags_hint")}
          />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

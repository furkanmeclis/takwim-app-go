"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useCreateCustomer } from "@/features/customers/hooks/use-customers";
import {
  createCustomerFormSchema,
  customerFormDefaults,
  customerFormToRequest,
  type CustomerFormValues,
} from "@/features/customers/schemas/customer-form";
import type { Customer } from "@/features/customers/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-customer-form";

/**
 * Adds a customer without leaving the booking form (name and contact; the
 * rest is edited on the customer's page). The same rules as the customer
 * form apply, so a phone already on file is refused by the API.
 */
export function CustomerQuickCreateDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (customer: Customer) => void;
}) {
  const { t } = useLocale();
  const create = useCreateCustomer();
  const schema = useMemo(() => createCustomerFormSchema(t), [t]);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("reservations.customer_dialog.title")}
      description={t("reservations.customer_dialog.description")}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("reservations.customer_dialog.submit")}
          </Button>
        </>
      }
    >
      {/* Opened from inside the booking form: React bubbles submit events
          through portals, so this form's submit must stop here. */}
      <div onSubmit={(event) => event.stopPropagation()}>
        {open ? (
          <AppForm<CustomerFormValues>
            id={FORM_ID}
            className="grid gap-4 sm:grid-cols-2"
            schema={schema}
            defaultValues={customerFormDefaults()}
            onSubmit={async (values) => {
              const created = await create.mutateAsync(
                customerFormToRequest(values),
              );
              onCreated(created);
              onOpenChange(false);
            }}
          >
            <AppInput
              name="first_name"
              label={t("customers.fields.first_name")}
            />
            <AppInput
              name="last_name"
              label={t("customers.fields.last_name")}
            />
            <AppInput
              name="phone"
              type="tel"
              inputMode="tel"
              label={t("customers.fields.phone")}
              description={t("customers.fields.phone_hint")}
            />
            <AppInput
              name="email"
              type="email"
              label={t("customers.fields.email")}
            />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

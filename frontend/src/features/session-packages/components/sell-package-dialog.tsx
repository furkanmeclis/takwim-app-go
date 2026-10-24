"use client";

import { useCallback, useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppForm,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
  type ComboboxOption,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useCashAccountOptions } from "@/features/cash/hooks/use-cash";
import { useCashAmount } from "@/features/cash/lib/format";
import { customersService } from "@/features/customers/services/customers.service";
import {
  MAX_NOTE,
  METHOD_LABEL_KEYS,
  PACKAGE_METHODS,
} from "@/features/session-packages/constants";
import {
  useSellPackage,
  useSessionPackages,
} from "@/features/session-packages/hooks/use-session-packages";
import type { SessionPackage } from "@/features/session-packages/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "sell-package-form";

type Translate = (key: string) => string;

function createSellSchema(t: Translate) {
  return z.object({
    package_uuid: z
      .string()
      .min(1, t("session_packages.validation.package_required")),
    customer_uuid: z
      .string()
      .min(1, t("session_packages.validation.customer_required")),
    branch_uuid: z
      .string()
      .min(1, t("session_packages.validation.branch_required")),
    method: z.enum(PACKAGE_METHODS as ["cash", "card", "transfer"]),
    cash_account_uuid: z
      .string()
      .min(1, t("session_packages.validation.account_required")),
    note: z.string().max(MAX_NOTE, t("session_packages.validation.note_max")),
  });
}

type SellFormValues = z.infer<ReturnType<typeof createSellSchema>>;

function SellFields({
  packages,
  fixedCustomer,
}: {
  packages: SessionPackage[];
  fixedCustomer?: { uuid: string; name: string };
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const money = useCashAmount();
  const { control } = useFormContext<SellFormValues>();
  const packageUuid = useWatch({ control, name: "package_uuid" });
  const branches = useAccessibleBranches();
  const accounts = useCashAccountOptions();
  const picked = packages.find((p) => p.uuid === packageUuid);

  const packageOptions = useMemo<ComboboxOption[]>(
    () =>
      packages.map((p) => ({
        value: p.uuid,
        label: p.name,
        description: t("session_packages.sell.package_option", {
          service: p.service.name,
          sessions: p.total_sessions,
          price: money(p.price, p.currency),
        }),
      })),
    [money, packages, t],
  );

  const branchOptions = useMemo(() => {
    const allowed = picked?.all_branches
      ? null
      : new Set((picked?.branches ?? []).map((b) => b.uuid));
    return (branches.data?.items ?? [])
      .filter((b) => !allowed || allowed.has(b.uuid))
      .map((b) => ({ value: b.uuid, label: b.name }));
  }, [branches.data?.items, picked]);

  const accountOptions = useMemo(
    () =>
      (accounts.data?.items ?? [])
        .filter(
          (a) =>
            a.status === "active" &&
            (!picked || a.currency === picked.currency),
        )
        .map((a) => ({
          value: a.uuid,
          label: a.branch_name ? `${a.name} · ${a.branch_name}` : a.name,
        })),
    [accounts.data?.items, picked],
  );

  const loadCustomers = useCallback(
    async (q: string) => {
      const page = await customersService.list(slug, {
        limit: 20,
        offset: 0,
        q: q || undefined,
      });
      return page.items.map((c) => ({
        value: c.uuid,
        label: c.full_name,
        description: c.phone_display || c.email || undefined,
      }));
    },
    [slug],
  );

  return (
    <>
      <AppCombobox
        name="package_uuid"
        label={t("session_packages.sell.fields.package")}
        placeholder={t("session_packages.sell.fields.package_placeholder")}
        searchPlaceholder={t("session_packages.sell.fields.package_search")}
        emptyText={t("session_packages.sell.fields.package_empty")}
        options={packageOptions}
        description={
          picked
            ? t("session_packages.sell.fields.package_hint", {
                value: money(picked.session_value, picked.currency),
                validity:
                  picked.validity_days != null
                    ? t("session_packages.validity_value", {
                        days: picked.validity_days,
                      })
                    : t("session_packages.validity_none"),
              })
            : undefined
        }
      />
      {fixedCustomer ? null : (
        <AppCombobox
          name="customer_uuid"
          label={t("session_packages.sell.fields.customer")}
          placeholder={t("session_packages.sell.fields.customer_placeholder")}
          searchPlaceholder={t("session_packages.sell.fields.customer_search")}
          emptyText={t("session_packages.sell.fields.customer_empty")}
          loadOptions={loadCustomers}
        />
      )}
      <AppSelect
        name="branch_uuid"
        label={t("session_packages.sell.fields.branch")}
        placeholder={t("session_packages.sell.fields.branch_placeholder")}
        description={t("session_packages.sell.fields.branch_hint")}
        options={branchOptions}
      />
      <AppToggleGroup
        name="method"
        label={t("session_packages.sell.fields.method")}
        options={PACKAGE_METHODS.map((value) => ({
          value,
          label: t(METHOD_LABEL_KEYS[value]),
        }))}
      />
      <AppSelect
        name="cash_account_uuid"
        label={t("session_packages.sell.fields.cash_account")}
        placeholder={t("session_packages.sell.fields.cash_account_placeholder")}
        description={
          accounts.isSuccess && accountOptions.length === 0
            ? t("session_packages.sell.fields.cash_account_none")
            : undefined
        }
        options={accountOptions}
      />
      <AppTextarea name="note" label={t("session_packages.sell.fields.note")} />
    </>
  );
}

/**
 * Sells a package to a customer: the full price is taken into a cash or
 * bank account at once (a payment, not revenue; the sessions become
 * revenue when they are used). The idempotency key makes a double submit
 * sell once.
 */
export function SellPackageDialog({
  open,
  onOpenChange,
  customer,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Sell to this customer (customer page); otherwise it is picked. */
  customer?: { uuid: string; name: string };
}) {
  const { t } = useLocale();
  const sell = useSellPackage();
  const packages = useSessionPackages(
    { limit: 100, offset: 0, sort: "name", is_active: "true" },
    open,
  );
  const schema = useMemo(() => createSellSchema(t), [t]);
  const [idempotencyKey, setIdempotencyKey] = useState(() =>
    crypto.randomUUID(),
  );
  const items = packages.data?.items ?? [];

  const close = (next: boolean) => {
    if (!next) setIdempotencyKey(crypto.randomUUID());
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("session_packages.sell.title")}
      description={
        customer
          ? t("session_packages.sell.description_customer", {
              name: customer.name,
            })
          : t("session_packages.sell.description")
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            disabled={sell.isPending || items.length === 0}
          >
            {t("session_packages.sell.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        packages.isSuccess && items.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("session_packages.sell.no_packages")}
            </AlertDescription>
          </Alert>
        ) : (
          <AppForm<SellFormValues>
            key={`${customer?.uuid ?? "any"}-${packages.isSuccess}`}
            id={FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              package_uuid: "",
              customer_uuid: customer?.uuid ?? "",
              branch_uuid: "",
              method: "cash",
              cash_account_uuid: "",
              note: "",
            }}
            onSubmit={async (values) => {
              await sell.mutateAsync({
                package_uuid: values.package_uuid,
                customer_uuid: values.customer_uuid,
                branch_uuid: values.branch_uuid,
                method: values.method,
                cash_account_uuid: values.cash_account_uuid,
                ...(values.note.trim() ? { note: values.note.trim() } : {}),
                idempotency_key: idempotencyKey,
              });
              close(false);
            }}
          >
            <SellFields packages={items} fixedCustomer={customer} />
          </AppForm>
        )
      ) : null}
    </AppDialog>
  );
}

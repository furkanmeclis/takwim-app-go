"use client";

import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { normalizeAmount, useCashAmount } from "@/features/cash/lib/format";
import { useServicesList } from "@/features/services/hooks/use-services";
import {
  MAX_DESCRIPTION,
  MAX_NAME,
  MAX_SESSIONS,
  MAX_VALIDITY_DAYS,
} from "@/features/session-packages/constants";
import { useSaveSessionPackage } from "@/features/session-packages/hooks/use-session-packages";
import { sessionValue } from "@/features/session-packages/lib/format";
import type {
  SessionPackage,
  SessionPackageInput,
} from "@/features/session-packages/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "session-package-form";
const AMOUNT = /^\d+([.,]\d{1,2})?$/;
const WHOLE = /^\d+$/;

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function createPackageSchema(t: Translate) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t("session_packages.validation.name_required"))
        .max(MAX_NAME, t("session_packages.validation.name_max")),
      service_uuid: z
        .string()
        .min(1, t("session_packages.validation.service_required")),
      total_sessions: z
        .string()
        .trim()
        .refine(
          (v) => WHOLE.test(v) && Number(v) >= 1 && Number(v) <= MAX_SESSIONS,
          t("session_packages.validation.sessions", { max: MAX_SESSIONS }),
        ),
      price: z
        .string()
        .trim()
        .refine((v) => AMOUNT.test(v), t("session_packages.validation.amount")),
      validity_days: z
        .string()
        .trim()
        .refine(
          (v) =>
            v === "" ||
            (WHOLE.test(v) && Number(v) >= 1 && Number(v) <= MAX_VALIDITY_DAYS),
          t("session_packages.validation.validity", {
            max: MAX_VALIDITY_DAYS,
          }),
        ),
      all_branches: z.boolean(),
      branch_uuids: z.array(z.string()),
      description: z
        .string()
        .max(MAX_DESCRIPTION, t("session_packages.validation.description_max")),
      is_active: z.boolean(),
    })
    .superRefine((values, ctx) => {
      if (!values.all_branches && values.branch_uuids.length === 0) {
        ctx.addIssue({
          code: "custom",
          path: ["branch_uuids"],
          message: t("session_packages.validation.branches_required"),
        });
      }
    });
}

type PackageFormValues = z.infer<ReturnType<typeof createPackageSchema>>;

function PackageFields() {
  const { t } = useLocale();
  const { organization } = useTenant();
  const money = useCashAmount();
  const { control } = useFormContext<PackageFormValues>();
  const allBranches = useWatch({ control, name: "all_branches" });
  const price = useWatch({ control, name: "price" });
  const total = useWatch({ control, name: "total_sessions" });
  const services = useServicesList({
    limit: 100,
    offset: 0,
    sort: "name",
    is_active: "true",
  });
  const branches = useAccessibleBranches();
  const serviceOptions = useMemo(
    () =>
      (services.data?.items ?? []).map((service) => ({
        value: service.uuid,
        label: service.name,
      })),
    [services.data?.items],
  );
  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: branch.name,
      })),
    [branches.data?.items],
  );
  const perSession = sessionValue(price, total);

  return (
    <>
      <AppInput
        name="name"
        label={t("session_packages.fields.name")}
        placeholder={t("session_packages.fields.name_placeholder")}
        className="sm:col-span-2"
      />
      <AppCombobox
        name="service_uuid"
        label={t("session_packages.fields.service")}
        description={t("session_packages.fields.service_hint")}
        placeholder={t("session_packages.fields.service_placeholder")}
        searchPlaceholder={t("session_packages.fields.service_search")}
        emptyText={t("session_packages.fields.service_empty")}
        options={serviceOptions}
        className="sm:col-span-2"
      />
      <AppInput
        name="total_sessions"
        inputMode="numeric"
        label={t("session_packages.fields.total_sessions")}
      />
      <AppInput
        name="price"
        inputMode="decimal"
        label={t("session_packages.fields.price", {
          currency: organization?.currency ?? "TRY",
        })}
        description={
          perSession
            ? t("session_packages.fields.price_hint", {
                value: money(perSession),
              })
            : undefined
        }
      />
      <AppInput
        name="validity_days"
        inputMode="numeric"
        label={t("session_packages.fields.validity_days")}
        description={t("session_packages.fields.validity_days_hint")}
      />
      <AppSwitch
        name="is_active"
        label={t("session_packages.fields.is_active")}
        description={t("session_packages.fields.is_active_hint")}
      />
      <AppSwitch
        name="all_branches"
        label={t("session_packages.fields.all_branches")}
        description={t("session_packages.fields.all_branches_hint")}
        className="sm:col-span-2"
      />
      {!allBranches ? (
        <AppMultiCombobox
          name="branch_uuids"
          label={t("session_packages.fields.branches")}
          placeholder={t("session_packages.fields.branches_placeholder")}
          emptyText={t("session_packages.fields.branches_empty")}
          options={branchOptions}
          className="sm:col-span-2"
        />
      ) : null}
      <AppTextarea
        name="description"
        label={t("session_packages.fields.description")}
        className="sm:col-span-2"
      />
    </>
  );
}

function toRequest(values: PackageFormValues): SessionPackageInput {
  return {
    name: values.name.trim(),
    description: values.description.trim(),
    service_uuid: values.service_uuid,
    total_sessions: Number(values.total_sessions),
    price: normalizeAmount(values.price),
    validity_days: values.validity_days ? Number(values.validity_days) : null,
    all_branches: values.all_branches,
    branch_uuids: values.all_branches ? [] : values.branch_uuids,
    is_active: values.is_active,
  };
}

/**
 * Create or change a package definition: N sessions of one service for a
 * price, the branches they can be used in and the validity. Packages
 * already sold keep the terms of the day they were sold.
 */
export function PackageDialog({
  pkg,
  open,
  onOpenChange,
}: {
  /** Package to edit; `null` creates a new one. */
  pkg: SessionPackage | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveSessionPackage();
  const schema = useMemo(() => createPackageSchema(t), [t]);
  const editing = Boolean(pkg);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        editing
          ? t("session_packages.dialog.edit_title")
          : t("session_packages.dialog.create_title")
      }
      description={t("session_packages.dialog.description")}
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
              ? t("session_packages.actions.save")
              : t("session_packages.actions.create")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<PackageFormValues>
          key={pkg?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            name: pkg?.name ?? "",
            service_uuid: pkg?.service.uuid ?? "",
            total_sessions: pkg ? String(pkg.total_sessions) : "",
            price: pkg?.price ?? "",
            validity_days:
              pkg?.validity_days != null ? String(pkg.validity_days) : "",
            all_branches: pkg?.all_branches ?? true,
            branch_uuids: pkg?.branches.map((branch) => branch.uuid) ?? [],
            description: pkg?.description ?? "",
            is_active: pkg?.is_active ?? true,
          }}
          onSubmit={async (values) => {
            await save.mutateAsync({
              uuid: pkg?.uuid,
              body: toRequest(values),
            });
            onOpenChange(false);
          }}
        >
          <PackageFields />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

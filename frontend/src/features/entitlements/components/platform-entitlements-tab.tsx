"use client";

import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  EntitlementLimitsCard,
  EntitlementUsageCard,
} from "@/features/entitlements/components/entitlement-overview";
import {
  ADDON_HINT_KEYS,
  ADDON_LABEL_KEYS,
  PLAN_LABEL_KEYS,
} from "@/features/entitlements/constants";
import {
  usePlatformEntitlements,
  useSetPlatformEntitlements,
} from "@/features/entitlements/hooks/use-entitlements";
import type {
  Addon,
  Plan,
  PlanCode,
  PlatformEntitlements,
} from "@/features/entitlements/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Translate = ReturnType<typeof useLocale>["t"];

/** An empty limit field means the plan default. */
function createSchema(t: Translate) {
  const limit = z
    .string()
    .trim()
    .refine((value) => value === "" || /^[1-9]\d{0,5}$/.test(value), {
      message: t("entitlements.validation.limit"),
    });
  return z.object({
    plan: z.enum(["starter", "growth", "enterprise"]),
    addons: z.record(z.string(), z.boolean()),
    branch_limit: limit,
    staff_limit: limit,
  });
}

type Values = z.infer<ReturnType<typeof createSchema>>;

function toValues(data: PlatformEntitlements): Values {
  return {
    plan: data.plan,
    addons: Object.fromEntries(
      data.catalog.addons.map((addon) => [addon, data.addons.includes(addon)]),
    ),
    branch_limit: data.overrides.branch_limit?.toString() ?? "",
    staff_limit: data.overrides.staff_limit?.toString() ?? "",
  };
}

const toLimit = (value: string) => (value === "" ? null : Number(value));

/** "Paket / özellikler": the organization's plan, add-ons and limit overrides. */
export function PlatformEntitlementsTab({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.organizations.write);
  const query = usePlatformEntitlements(uuid);
  const save = useSetPlatformEntitlements(uuid);
  const schema = useMemo(() => createSchema(t), [t]);

  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const data = query.data;
  const planDefault = (code: PlanCode) =>
    data.catalog.plans.find((plan) => plan.code === code);
  const limitText = (limit: Plan["branch_limit"] | undefined) =>
    limit === null || limit === undefined
      ? t("entitlements.unlimited")
      : String(limit);
  const planOptions = data.catalog.plans.map((plan) => ({
    value: plan.code,
    label: t("entitlements.plan_option", {
      name: t(PLAN_LABEL_KEYS[plan.code]),
      branches: limitText(plan.branch_limit),
      staff: limitText(plan.staff_limit),
    }),
  }));

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <EntitySectionCard title={t("entitlements.settings_title")}>
        <AppForm<Values>
          key={JSON.stringify(toValues(data))}
          className="space-y-4"
          schema={schema}
          defaultValues={toValues(data)}
          onSubmit={async (values) => {
            await save.mutateAsync({
              plan: values.plan,
              addons: data.catalog.addons.filter(
                (addon: Addon) => values.addons[addon],
              ),
              branch_limit: toLimit(values.branch_limit),
              staff_limit: toLimit(values.staff_limit),
            });
          }}
        >
          {(form) => {
            const selected = planDefault(form.watch("plan"));
            return (
              <>
                <AppSelect
                  name="plan"
                  label={t("entitlements.plan")}
                  options={planOptions}
                  disabled={!canWrite}
                />
                {data.catalog.addons.map((addon) => (
                  <AppSwitch
                    key={addon}
                    name={`addons.${addon}`}
                    label={t(ADDON_LABEL_KEYS[addon])}
                    description={t(ADDON_HINT_KEYS[addon])}
                    disabled={!canWrite}
                  />
                ))}
                <div className="grid gap-4 sm:grid-cols-2">
                  <AppInput
                    name="branch_limit"
                    inputMode="numeric"
                    label={t("entitlements.branch_limit")}
                    placeholder={limitText(selected?.branch_limit)}
                    description={t("entitlements.limit_hint")}
                    disabled={!canWrite}
                  />
                  <AppInput
                    name="staff_limit"
                    inputMode="numeric"
                    label={t("entitlements.staff_limit")}
                    placeholder={limitText(selected?.staff_limit)}
                    description={t("entitlements.limit_hint")}
                    disabled={!canWrite}
                  />
                </div>
                <p className="text-muted-foreground text-xs">
                  {t("entitlements.lower_hint")}
                </p>
                {canWrite ? (
                  <FormActions>
                    <Button
                      type="submit"
                      disabled={
                        !form.formState.isDirty || form.formState.isSubmitting
                      }
                    >
                      {t("organizations.actions.save")}
                    </Button>
                  </FormActions>
                ) : null}
              </>
            );
          }}
        </AppForm>
      </EntitySectionCard>
      <div className="space-y-6">
        <EntitlementLimitsCard summary={data} />
        <EntitlementUsageCard summary={data} />
      </div>
    </div>
  );
}

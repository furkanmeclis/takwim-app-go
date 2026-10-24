"use client";

import { useEffect, useMemo, useState } from "react";
import type { FieldErrors, UseFormReturn } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { AppForm, FormActions } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  tabOfPath,
  type SettingsTab,
} from "@/features/tenant-settings/constants";
import { BookingSection } from "@/features/tenant-settings/components/booking-section";
import { OrganizationProfileCard } from "@/features/tenant-settings/components/organization-profile-card";
import { SettingsReadiness } from "@/features/tenant-settings/components/settings-readiness";
import {
  ChannelsSection,
  CompanySection,
  PoliciesSection,
  ReservationSection,
  TaxSection,
} from "@/features/tenant-settings/components/settings-sections";
import { SettingsVersionsTab } from "@/features/tenant-settings/components/settings-versions-tab";
import {
  useSaveTenantSettings,
  useTenantSettings,
} from "@/features/tenant-settings/hooks/use-tenant-settings";
import {
  createSettingsSchema,
  formToSaveRequest,
  settingsToForm,
  type SettingsFormValues,
} from "@/features/tenant-settings/schemas/settings-form";
import type { OrganizationSettingsVersion } from "@/features/tenant-settings/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Tab = SettingsTab | "history";

const FORM_TABS: { id: SettingsTab; labelKey: string }[] = [
  { id: "company", labelKey: "tenant_settings.tabs.company" },
  { id: "reservation", labelKey: "tenant_settings.tabs.reservation" },
  { id: "booking", labelKey: "tenant_settings.tabs.booking" },
  { id: "policies", labelKey: "tenant_settings.tabs.policies" },
  { id: "channels", labelKey: "tenant_settings.tabs.channels" },
  { id: "tax", labelKey: "tenant_settings.tabs.tax" },
];

/** Dot paths of every error in a (nested) RHF error tree. */
function errorPaths(errors: FieldErrors, prefix = ""): string[] {
  const out: string[] = [];
  for (const [key, value] of Object.entries(errors)) {
    if (!value || typeof value !== "object") continue;
    const path = prefix ? `${prefix}.${key}` : key;
    if ("message" in value && typeof value.message === "string") {
      out.push(path);
    } else {
      out.push(...errorPaths(value as FieldErrors, path));
    }
  }
  return out;
}

function SettingsTabs({
  form,
  current,
  canWrite,
  tab,
  setTab,
  conflict,
  onReload,
}: {
  form: UseFormReturn<SettingsFormValues>;
  current: OrganizationSettingsVersion;
  canWrite: boolean;
  tab: Tab;
  setTab: (tab: Tab) => void;
  conflict: boolean;
  onReload: () => void;
}) {
  const { t, locale } = useLocale();
  const { errors, submitCount, isDirty, isSubmitting } = form.formState;
  const paths = errorPaths(errors);
  const tabsWithErrors = new Set(paths.map(tabOfPath));
  const firstErrorTab = paths.length ? tabOfPath(paths[0]) : null;

  // After a failed submit (client or server validation) open the first tab
  // with an error.
  useEffect(() => {
    if (submitCount > 0 && firstErrorTab) setTab(firstErrorTab);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [submitCount, firstErrorTab]);

  const disabled = !canWrite;
  return (
    <>
      <TabsList className="h-auto w-full flex-wrap justify-start sm:w-fit">
        {FORM_TABS.map((item) => (
          <TabsTrigger key={item.id} value={item.id} className="flex-none">
            {t(item.labelKey)}
            {tabsWithErrors.has(item.id) ? (
              <span
                className="bg-destructive size-1.5 rounded-full"
                aria-label={t("tenant_settings.tab_has_errors")}
              />
            ) : null}
          </TabsTrigger>
        ))}
        <TabsTrigger value="history" className="flex-none">
          {t("tenant_settings.tabs.history")}
        </TabsTrigger>
      </TabsList>

      {conflict ? (
        <Alert variant="destructive">
          <AlertTitle>{t("tenant_settings.conflict.title")}</AlertTitle>
          <AlertDescription>
            <p>{t("tenant_settings.conflict.description")}</p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={onReload}
            >
              {t("tenant_settings.conflict.reload")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {tab !== "history" ? (
        <SettingsReadiness
          ready={current.ready_for_online_sales}
          missing={current.missing}
          onFix={() => setTab("reservation")}
        />
      ) : null}

      <TabsContent
        value="company"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <CompanySection disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="reservation"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <ReservationSection disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="booking"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <BookingSection disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="policies"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <PoliciesSection disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="channels"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <ChannelsSection disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="tax"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <TaxSection disabled={disabled} />
      </TabsContent>

      {canWrite && tab !== "history" ? (
        <FormActions className="justify-between">
          <p className="text-muted-foreground text-sm">
            {isDirty
              ? t("tenant_settings.save_bar.dirty")
              : current.version > 0 && current.created_at
                ? t("tenant_settings.save_bar.saved", {
                    version: current.version,
                    date: datetime(current.created_at, undefined, locale),
                  })
                : t("tenant_settings.save_bar.never_saved")}
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!isDirty || isSubmitting}
              onClick={() => form.reset()}
            >
              {t("tenant_settings.save_bar.discard")}
            </Button>
            <Button type="submit" disabled={!isDirty || isSubmitting}>
              {t("tenant_settings.save_bar.save")}
            </Button>
          </div>
        </FormActions>
      ) : null}
    </>
  );
}

/**
 * Organization settings (TEC-690): one versioned document edited in tabs
 * (company, reservation & payment, booking flow & form, policies, sales
 * channels, tax) with a
 * sticky save bar; every save stores a new immutable version, browsable and
 * comparable in the history tab.
 */
export function TenantSettingsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const query = useTenantSettings();
  const save = useSaveTenantSettings();
  const schema = useMemo(() => createSettingsSchema(t), [t]);
  const [tab, setTab] = useState<Tab>("company");
  const [conflict, setConflict] = useState(false);
  const current = query.data;
  const defaults = useMemo(
    () => (current ? settingsToForm(current.settings) : undefined),
    [current],
  );

  return (
    <EntityPage
      title={t("tenant_settings.title")}
      description={t("tenant_settings.description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_settings.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("tenant_settings.title") },
      ]}
    >
      <div className="space-y-6">
        <OrganizationProfileCard />
        {query.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : query.isError || !current || !defaults ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("tenant_settings.load_failed")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : (
          <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)}>
            <AppForm<SettingsFormValues>
              key={current.version}
              className="space-y-4"
              schema={schema}
              defaultValues={defaults}
              onSubmit={async (values) => {
                setConflict(false);
                try {
                  await save.mutateAsync(
                    formToSaveRequest(values, current.version),
                  );
                } catch (error) {
                  if (isApiError(error) && error.status === 409) {
                    setConflict(true);
                  }
                  throw error;
                }
              }}
            >
              {(form) => (
                <SettingsTabs
                  form={form}
                  current={current}
                  canWrite={canWrite}
                  tab={tab}
                  setTab={setTab}
                  conflict={conflict}
                  onReload={() => {
                    setConflict(false);
                    void query.refetch();
                  }}
                />
              )}
            </AppForm>
            <TabsContent value="history">
              <SettingsVersionsTab currentVersion={current.version} />
            </TabsContent>
          </Tabs>
        )}
      </div>
    </EntityPage>
  );
}

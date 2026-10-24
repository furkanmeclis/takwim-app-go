"use client";

import { Check, Copy, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useState, useSyncExternalStore } from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import {
  AppDatePicker,
  AppInput,
  AppRadioGroup,
  AppSelect,
  FormSection,
} from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  usePublicBranches,
  usePublicServices,
  usePublicStaff,
} from "@/features/business-page/hooks/use-public-booking";
import { useTenantSettings } from "@/features/tenant-settings/hooks/use-tenant-settings";
import {
  embedSnippet,
  preselectLink,
  previewPath,
  type EmbedCodeOptions,
  type EmbedMode,
} from "@/features/tenant-settings/lib/embed-code";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Select value of "no pre-selection" (Radix Select has no empty value). */
const ANY = "any";

type Values = {
  mode: EmbedMode;
  view: "compact" | "full";
  branch: string;
  service: string;
  staff: string;
  date: string;
  label: string;
};

const noop = () => () => {};

function CodeBlock({ title, code }: { title: string; code: string }) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      appToast.success(t("tenant_settings.embed_code.copied"));
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      appToast.error(t("tenant_settings.embed_code.copy_failed"));
    }
  };
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-medium">{title}</p>
        <Button type="button" variant="outline" size="sm" onClick={copy}>
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
          {t("tenant_settings.embed_code.copy")}
        </Button>
      </div>
      <pre className="bg-muted overflow-x-auto rounded-lg border p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap">
        {code}
      </pre>
    </div>
  );
}

function Generator({ slug }: { slug: string }) {
  const { t } = useLocale();
  const origin = useSyncExternalStore(
    noop,
    () => window.location.origin,
    () => "",
  );
  const form = useForm<Values>({
    defaultValues: {
      mode: "inline",
      view: "compact",
      branch: ANY,
      service: ANY,
      staff: ANY,
      date: "",
      label: t("tenant_settings.embed_code.default_button"),
    },
  });
  const values = useWatch({ control: form.control }) as Values;
  const branchesQuery = usePublicBranches(slug);
  const branches = branchesQuery.data ?? [];
  const branch =
    values.branch !== ANY
      ? values.branch
      : branches.length === 1
        ? branches[0]!.uuid
        : "";
  const servicesQuery = usePublicServices(slug, branch);
  const staffQuery = usePublicStaff(slug, branch);
  const services = servicesQuery.data ?? [];
  const service = services.find((s) => s.uuid === values.service);
  // People of the picked service, else everyone shown online in the branch.
  const staff = (staffQuery.data ?? []).filter(
    (p) => !service || service.staff.some((s) => s.staff_uuid === p.uuid),
  );

  const pick = (value: string, list: { uuid: string }[]) =>
    value !== ANY && list.some((x) => x.uuid === value) ? value : undefined;
  const options: EmbedCodeOptions = {
    origin,
    slug,
    mode: values.mode,
    view: values.view,
    branch: pick(values.branch, branches),
    service: branch ? pick(values.service, services) : undefined,
    staff: branch ? pick(values.staff, staff) : undefined,
    date: values.date || undefined,
    label: values.label ?? "",
  };
  const anyOption = { value: ANY, label: t("tenant_settings.embed_code.any") };

  return (
    <FormProvider {...form}>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)] lg:items-start">
        <FormSection
          title={t("tenant_settings.embed_code.options_title")}
          description={t("tenant_settings.embed_code.options_description")}
        >
          <AppRadioGroup
            name="mode"
            label={t("tenant_settings.embed_code.mode")}
            options={[
              {
                value: "inline",
                label: t("tenant_settings.embed_code.mode_inline"),
              },
              {
                value: "button",
                label: t("tenant_settings.embed_code.mode_button"),
              },
            ]}
          />
          {values.mode === "button" ? (
            <AppInput
              name="label"
              label={t("tenant_settings.embed_code.button_label")}
              maxLength={60}
            />
          ) : null}
          <AppRadioGroup
            name="view"
            label={t("tenant_settings.embed_code.view")}
            options={[
              {
                value: "compact",
                label: t("tenant_settings.embed_code.view_compact"),
              },
              {
                value: "full",
                label: t("tenant_settings.embed_code.view_full"),
              },
            ]}
          />
          <AppSelect
            name="branch"
            label={t("tenant_settings.embed_code.branch")}
            options={[
              anyOption,
              ...branches.map((b) => ({ value: b.uuid, label: b.name })),
            ]}
            disabled={branchesQuery.isLoading}
          />
          <AppSelect
            name="service"
            label={t("tenant_settings.embed_code.service")}
            description={
              branch ? undefined : t("tenant_settings.embed_code.pick_branch")
            }
            options={[
              anyOption,
              ...services.map((s) => ({ value: s.uuid, label: s.name })),
            ]}
            disabled={!branch}
          />
          <AppSelect
            name="staff"
            label={t("tenant_settings.embed_code.staff")}
            options={[
              anyOption,
              ...staff.map((p) => ({ value: p.uuid, label: p.name })),
            ]}
            disabled={!branch}
          />
          <AppDatePicker
            name="date"
            label={t("tenant_settings.embed_code.date")}
            description={t("tenant_settings.embed_code.date_hint")}
          />
        </FormSection>

        <div className="space-y-4">
          <FormSection
            title={t("tenant_settings.embed_code.code_title")}
            description={t(
              values.mode === "button"
                ? "tenant_settings.embed_code.code_button_description"
                : "tenant_settings.embed_code.code_inline_description",
            )}
          >
            {origin ? (
              <>
                <CodeBlock
                  title={t("tenant_settings.embed_code.snippet")}
                  code={embedSnippet(options)}
                />
                <CodeBlock
                  title={t("tenant_settings.embed_code.link")}
                  code={preselectLink(options)}
                />
              </>
            ) : null}
          </FormSection>
          <FormSection
            title={t("tenant_settings.embed_code.preview_title")}
            description={t("tenant_settings.embed_code.preview_description")}
          >
            <iframe
              key={previewPath(options)}
              src={previewPath(options)}
              title={t("tenant_settings.embed_code.preview_title")}
              className="bg-background h-[640px] w-full rounded-lg border"
            />
          </FormSection>
        </div>
      </div>
    </FormProvider>
  );
}

/**
 * Embed code generator (TEC-908): the inline booking area or the "book"
 * button for the business's own site, with a pre-selection, the direct
 * link and a live preview. The code works only on the sites listed in the
 * settings and while the `widget` channel is on.
 */
export function EmbedCodePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const settings = useTenantSettings();
  const current = settings.data?.settings;
  const origins = current?.embed?.allowed_origins ?? [];
  const widgetOn =
    current?.sales_channels.find((c) => c.code === "widget")?.is_active ??
    false;

  return (
    <EntityPage
      title={t("tenant_settings.embed_code.title")}
      description={t("tenant_settings.embed_code.description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_settings.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("tenant_settings.title"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("tenant_settings.embed_code.title") },
      ]}
    >
      {settings.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : settings.isError || !current ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("tenant_settings.load_failed")}
          onRetry={() => void settings.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <div className="space-y-4">
          {!widgetOn || origins.length === 0 ? (
            <Alert>
              <TriangleAlert />
              <AlertTitle>
                {t("tenant_settings.embed_code.not_ready_title")}
              </AlertTitle>
              <AlertDescription>
                <ul className="list-disc ps-4">
                  {!widgetOn ? (
                    <li>{t("tenant_settings.embed_code.widget_off")}</li>
                  ) : null}
                  {origins.length === 0 ? (
                    <li>{t("tenant_settings.embed_code.no_origins")}</li>
                  ) : null}
                </ul>
                <Button asChild size="sm" variant="outline" className="mt-2">
                  <Link href={routes.tenant.settings.root(slug)}>
                    {t("tenant_settings.embed_code.open_settings")}
                  </Link>
                </Button>
              </AlertDescription>
            </Alert>
          ) : (
            <Alert>
              <Check />
              <AlertTitle>
                {t("tenant_settings.embed_code.ready_title")}
              </AlertTitle>
              <AlertDescription>{origins.join(", ")}</AlertDescription>
            </Alert>
          )}
          <Generator slug={slug} />
        </div>
      )}
    </EntityPage>
  );
}

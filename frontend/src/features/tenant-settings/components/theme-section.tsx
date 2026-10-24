"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ImageUp, Paintbrush, Trash2, X } from "lucide-react";
import { useRef, useState } from "react";
import { Controller, get, useFormContext, useWatch } from "react-hook-form";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  AppInput,
  AppSelect,
  AppTextarea,
  FormFieldShell,
  FormSection,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Hero } from "@/features/business-page/components/business-page";
import { BusinessTheme } from "@/features/business-page/components/business-theme";
import {
  HEX_COLOR,
  THEME_FONTS,
  THEME_FONT_LABEL_KEYS,
  resolveTheme,
  type ColorMode,
} from "@/features/business-page/lib/theme";
import type { PublicBusiness } from "@/features/business-page/types";
import {
  PROFILE_PATH,
  profileKey,
  type OrganizationProfile,
} from "@/features/tenant-settings/components/organization-profile-card";
import {
  MAX_THEME_DESCRIPTION,
  MAX_THEME_HEADLINE,
} from "@/features/tenant-settings/constants";
import type { SettingsFormValues } from "@/features/tenant-settings/schemas/settings-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type SectionProps = { disabled?: boolean };

/** Ready-made swatches; a hex value typed by hand works as well. */
const SWATCHES = [
  "#022c81",
  "#1d4ed8",
  "#0e7490",
  "#047857",
  "#4d7c0f",
  "#b45309",
  "#c2410c",
  "#be123c",
  "#a21caf",
  "#6d28d9",
  "#374151",
  "#111827",
] as const;

const ACCENT_SWATCHES = [
  "#dbeafe",
  "#cffafe",
  "#d1fae5",
  "#ecfccb",
  "#fef3c7",
  "#ffedd5",
  "#ffe4e6",
  "#fae8ff",
  "#ede9fe",
  "#f1f5f9",
  "#fde68a",
  "#e2e8f0",
] as const;

/** Static label keys (no template keys, see check-i18n). */
const FALLBACK_FIELD_KEYS = {
  primary_color: "tenant_settings.theme.primary_color",
  accent_color: "tenant_settings.theme.accent_color",
} as const;
const MODE_KEYS: Record<ColorMode, string> = {
  light: "tenant_settings.theme.mode_light",
  dark: "tenant_settings.theme.mode_dark",
};

const COVER_ACCEPT = "image/png,image/jpeg,image/webp";
const COVER_MAX_BYTES = 5 * 1024 * 1024;

/**
 * Colour field: a swatch grid behind a colour button plus the hex value as
 * text. Empty means the app's neutral colour.
 */
function ColorField({
  name,
  label,
  description,
  swatches,
  disabled,
}: {
  name: "theme.primary_color" | "theme.accent_color";
  label: string;
  description?: string;
  swatches: readonly string[];
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<SettingsFormValues>();
  const error = get(errors, name)?.message as string | undefined;
  return (
    <FormFieldShell
      name={name}
      label={label}
      description={description}
      error={error}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => {
          const valid = HEX_COLOR.test(field.value);
          return (
            <div className="flex items-center gap-2">
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    disabled={disabled}
                    aria-label={t("tenant_settings.theme.pick_color")}
                    className="shrink-0"
                  >
                    <span
                      aria-hidden
                      className={cn(
                        "size-5 rounded-full border",
                        !valid && "bg-muted",
                      )}
                      style={valid ? { backgroundColor: field.value } : {}}
                    />
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="start" className="w-auto p-3">
                  <div className="grid grid-cols-6 gap-2">
                    {swatches.map((swatch) => (
                      <button
                        key={swatch}
                        type="button"
                        aria-label={swatch}
                        aria-pressed={field.value === swatch}
                        className={cn(
                          "size-7 rounded-full border transition-transform hover:scale-110",
                          field.value === swatch &&
                            "ring-ring ring-2 ring-offset-2",
                        )}
                        style={{ backgroundColor: swatch }}
                        onClick={() => field.onChange(swatch)}
                      />
                    ))}
                  </div>
                </PopoverContent>
              </Popover>
              <Input
                id={name}
                value={field.value}
                placeholder={t("tenant_settings.theme.color_placeholder")}
                aria-invalid={Boolean(error)}
                disabled={disabled}
                className="font-mono"
                maxLength={7}
                spellCheck={false}
                onChange={(event) => field.onChange(event.target.value)}
                onBlur={field.onBlur}
              />
              {field.value && !disabled ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("tenant_settings.theme.clear_color")}
                  onClick={() => field.onChange("")}
                >
                  <X className="size-4" />
                </Button>
              ) : null}
            </div>
          );
        }}
      />
    </FormFieldShell>
  );
}

/**
 * Cover image of the page: uploaded through the API like the logo and shown
 * from the API-owned `cover_url` only. Saved on its own, outside the
 * settings document.
 */
function CoverCard({ disabled }: SectionProps) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const query = useQuery({
    queryKey: profileKey(slug),
    queryFn: () =>
      tenantRequest<OrganizationProfile>(slug, "GET", PROFILE_PATH),
  });
  const refresh = (profile: OrganizationProfile) => {
    queryClient.setQueryData(profileKey(slug), profile);
  };
  const upload = useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("cover", file);
      return tenantFormRequest<OrganizationProfile>(
        slug,
        "PUT",
        `${PROFILE_PATH}/cover`,
        form,
      );
    },
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.theme.cover_uploaded"));
      refresh(profile);
    },
  });
  const remove = useMutation({
    mutationFn: () =>
      tenantRequest<OrganizationProfile>(
        slug,
        "DELETE",
        `${PROFILE_PATH}/cover`,
      ),
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.theme.cover_removed"));
      refresh(profile);
      setConfirmRemove(false);
    },
  });
  const profile = query.data;
  const src = resolveLogoSrc(assertServiceMediaURL(profile?.cover_url));

  return (
    <FormSection
      title={t("tenant_settings.theme.cover_title")}
      description={t("tenant_settings.theme.cover_desc")}
    >
      {!profile ? (
        <Skeleton className="aspect-[3/1] w-full rounded-lg" />
      ) : (
        <div className="flex flex-col gap-3">
          {src ? (
            // API-owned image stream; next/image would need the API host.
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={src}
              alt=""
              className="aspect-[3/1] w-full rounded-lg border object-cover"
            />
          ) : (
            <div className="from-primary/15 via-muted to-background text-muted-foreground flex aspect-[3/1] w-full items-center justify-center rounded-lg border bg-gradient-to-br text-sm">
              {t("tenant_settings.theme.cover_empty")}
            </div>
          )}
          {!disabled ? (
            <div className="flex flex-wrap items-center gap-2">
              <Input
                ref={fileInput}
                type="file"
                accept={COVER_ACCEPT}
                className="hidden"
                aria-label={t("tenant_settings.theme.cover_upload")}
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  event.target.value = "";
                  if (!file) return;
                  if (file.size > COVER_MAX_BYTES) {
                    appToast.error(t("tenant_settings.theme.cover_too_large"));
                    return;
                  }
                  upload.mutate(file);
                }}
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={upload.isPending}
                onClick={() => fileInput.current?.click()}
              >
                <ImageUp className="size-4" />
                {profile.cover_url
                  ? t("tenant_settings.theme.cover_replace")
                  : t("tenant_settings.theme.cover_upload")}
              </Button>
              {profile.cover_url ? (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="text-destructive"
                  onClick={() => setConfirmRemove(true)}
                >
                  <Trash2 className="size-4" />
                  {t("tenant_settings.theme.cover_remove")}
                </Button>
              ) : null}
              <p className="text-muted-foreground text-xs">
                {t("tenant_settings.theme.cover_hint")}
              </p>
            </div>
          ) : null}
        </div>
      )}
      <ConfirmDialog
        open={confirmRemove}
        title={t("tenant_settings.theme.cover_remove_title")}
        description={t("tenant_settings.theme.cover_remove_description")}
        confirmLabel={t("tenant_settings.theme.cover_remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() => remove.mutate()}
      />
    </FormSection>
  );
}

/**
 * Live preview of the page hero and a few controls with the values being
 * edited, in light or dark mode. Colours that fail the contrast check in the
 * shown mode are listed, because the page keeps the neutral colour there.
 */
function ThemePreview() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const [mode, setMode] = useState<ColorMode>("light");
  const values = useWatch<SettingsFormValues, "theme">({ name: "theme" });
  const profile = useQuery({
    queryKey: profileKey(slug),
    queryFn: () =>
      tenantRequest<OrganizationProfile>(slug, "GET", PROFILE_PATH),
  }).data;
  const theme: PublicBusiness["theme"] = {
    primary_color: HEX_COLOR.test(values.primary_color)
      ? values.primary_color
      : "",
    accent_color: HEX_COLOR.test(values.accent_color)
      ? values.accent_color
      : "",
    font: values.font,
    headline: values.headline,
    description: values.description,
    cover_url: profile?.cover_url ?? null,
  };
  const resolved = resolveTheme(theme);
  const fallbacks = resolved[mode].fallbacks;
  const business = {
    name: organization?.name ?? "",
    logo_url: profile?.logo_url ?? null,
    sector: organization?.sector ?? "hairdresser",
    theme,
  } as PublicBusiness;
  const cover = resolveLogoSrc(assertServiceMediaURL(profile?.cover_url));

  return (
    <FormSection
      title={t("tenant_settings.theme.preview_title")}
      description={t("tenant_settings.theme.preview_desc")}
    >
      <Tabs value={mode} onValueChange={(value) => setMode(value as ColorMode)}>
        <TabsList>
          <TabsTrigger value="light">
            {t("tenant_settings.theme.mode_light")}
          </TabsTrigger>
          <TabsTrigger value="dark">
            {t("tenant_settings.theme.mode_dark")}
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <div
        className={cn(
          mode,
          "bg-background text-foreground overflow-hidden rounded-xl border",
        )}
      >
        <BusinessTheme
          scope={`preview-${mode}`}
          theme={theme}
          mode={mode}
          className="bg-muted/40 space-y-4 p-4"
        >
          <Hero business={business} branches={[]} cover={cover} />
          <div className="bg-card flex flex-wrap items-center gap-2 rounded-xl border p-4">
            <Button type="button" size="sm">
              {t("tenant_settings.theme.preview_button")}
            </Button>
            <Button type="button" size="sm" variant="outline">
              {t("tenant_settings.theme.preview_secondary")}
            </Button>
            <span className="bg-accent text-accent-foreground rounded-md px-2 py-1 text-xs font-medium">
              {t("tenant_settings.theme.preview_selected")}
            </span>
            <a
              href="#"
              onClick={(e) => e.preventDefault()}
              className="text-primary text-sm underline underline-offset-4"
            >
              {t("tenant_settings.theme.preview_link")}
            </a>
          </div>
        </BusinessTheme>
      </div>
      {fallbacks.length > 0 ? (
        <Alert>
          <Paintbrush />
          <AlertDescription>
            {t("tenant_settings.theme.contrast_warning", {
              fields: fallbacks
                .map((field) => t(FALLBACK_FIELD_KEYS[field]))
                .join(", "),
              mode: t(MODE_KEYS[mode]).toLocaleLowerCase("tr"),
            })}
          </AlertDescription>
        </Alert>
      ) : null}
    </FormSection>
  );
}

/**
 * Tema: colours, font and welcome texts of the business page and the
 * embedded screen (TEC-966), the cover image and a live preview.
 */
export function ThemeSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  const fontOptions = THEME_FONTS.map((font) => ({
    value: font,
    label: t(THEME_FONT_LABEL_KEYS[font]),
  }));
  return (
    <div className="space-y-4">
      <FormSection
        title={t("tenant_settings.sections.theme")}
        description={t("tenant_settings.sections.theme_desc")}
        columns={2}
      >
        <ColorField
          name="theme.primary_color"
          label={t("tenant_settings.theme.primary_color")}
          description={t("tenant_settings.theme.primary_color_hint")}
          swatches={SWATCHES}
          disabled={disabled}
        />
        <ColorField
          name="theme.accent_color"
          label={t("tenant_settings.theme.accent_color")}
          description={t("tenant_settings.theme.accent_color_hint")}
          swatches={ACCENT_SWATCHES}
          disabled={disabled}
        />
        <AppSelect
          name="theme.font"
          label={t("tenant_settings.theme.font")}
          description={t("tenant_settings.theme.font_hint")}
          options={fontOptions}
          disabled={disabled}
          className="sm:col-span-2"
        />
        <AppInput
          name="theme.headline"
          label={t("tenant_settings.theme.headline")}
          description={t("tenant_settings.theme.headline_hint", {
            max: MAX_THEME_HEADLINE,
          })}
          maxLength={MAX_THEME_HEADLINE}
          disabled={disabled}
          className="sm:col-span-2"
        />
        <AppTextarea
          name="theme.description"
          label={t("tenant_settings.theme.description")}
          description={t("tenant_settings.theme.description_hint", {
            max: MAX_THEME_DESCRIPTION,
          })}
          rows={3}
          maxLength={MAX_THEME_DESCRIPTION}
          disabled={disabled}
          className="sm:col-span-2"
        />
      </FormSection>
      <CoverCard disabled={disabled} />
      <ThemePreview />
    </div>
  );
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, ImageUp, Trash2 } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { z } from "zod";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput } from "@/components/forms";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantKeys } from "@/features/tenant/services/tenant.service";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type OrganizationProfile = components["schemas"]["OrganizationProfile"];

const PATH = "/v1/tenant/organization";
const ACCEPT = "image/png,image/jpeg,image/webp,image/svg+xml";
const MAX_BYTES = 2 * 1024 * 1024;

const profileKey = (slug: string) => ["tenant", slug, "organization"] as const;

function createNameSchema(t: (key: string) => string) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("tenant_settings.profile.validation.name_required"))
      .max(255, t("tenant_settings.profile.validation.name_too_long")),
  });
}

type NameValues = z.infer<ReturnType<typeof createNameSchema>>;

/**
 * Organization profile (TEC-671): display name and logo. Company / billing
 * details stay in the settings document below. The logo is uploaded through
 * the API and shown from the API-owned `logo_url` only.
 */
export function OrganizationProfileCard() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { hydrateProfile } = useAuth();
  const queryClient = useQueryClient();
  const canWrite = can(permissions.tenant.settings.write);
  const fileInput = useRef<HTMLInputElement>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const schema = useMemo(() => createNameSchema(t), [t]);

  const query = useQuery({
    queryKey: profileKey(slug),
    queryFn: () => tenantRequest<OrganizationProfile>(slug, "GET", PATH),
  });

  const refresh = (profile: OrganizationProfile) => {
    queryClient.setQueryData(profileKey(slug), profile);
    void queryClient.invalidateQueries({ queryKey: tenantKeys.context(slug) });
    // Sidebar, switcher and organization picker read `/me` organizations.
    void hydrateProfile();
  };

  const rename = useMutation({
    mutationFn: (values: NameValues) =>
      tenantRequest<OrganizationProfile>(slug, "PATCH", PATH, {
        body: { name: values.name.trim() },
      }),
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.profile.toast.renamed"));
      refresh(profile);
    },
  });
  const upload = useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("logo", file);
      return tenantFormRequest<OrganizationProfile>(
        slug,
        "PUT",
        `${PATH}/logo`,
        form,
      );
    },
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.profile.toast.logo_uploaded"));
      refresh(profile);
    },
  });
  const removeLogo = useMutation({
    mutationFn: () =>
      tenantRequest<OrganizationProfile>(slug, "DELETE", `${PATH}/logo`),
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.profile.toast.logo_removed"));
      refresh(profile);
      setConfirmRemove(false);
    },
  });

  const profile = query.data;
  const logoSrc = resolveLogoSrc(assertServiceMediaURL(profile?.logo_url));

  return (
    <EntitySectionCard title={t("tenant_settings.profile.title")}>
      {!profile ? (
        <Skeleton className="h-20 w-full" />
      ) : (
        <div className="flex flex-col gap-6 md:flex-row md:items-start">
          <div className="flex items-center gap-4">
            <Avatar className="size-20 rounded-xl border">
              {logoSrc ? (
                <AvatarImage
                  src={logoSrc}
                  alt=""
                  className="object-contain p-1"
                />
              ) : null}
              <AvatarFallback className="rounded-xl">
                <Building2 className="text-muted-foreground size-8" />
              </AvatarFallback>
            </Avatar>
            {canWrite ? (
              <div className="flex flex-col gap-2">
                <Input
                  ref={fileInput}
                  type="file"
                  accept={ACCEPT}
                  className="hidden"
                  aria-label={t("tenant_settings.profile.logo_upload")}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    event.target.value = "";
                    if (!file) return;
                    if (file.size > MAX_BYTES) {
                      appToast.error(
                        t("tenant_settings.profile.logo_too_large"),
                      );
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
                  {profile.logo_url
                    ? t("tenant_settings.profile.logo_replace")
                    : t("tenant_settings.profile.logo_upload")}
                </Button>
                {profile.logo_url ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="text-destructive"
                    onClick={() => setConfirmRemove(true)}
                  >
                    <Trash2 className="size-4" />
                    {t("tenant_settings.profile.logo_remove")}
                  </Button>
                ) : null}
                <p className="text-muted-foreground text-xs">
                  {t("tenant_settings.profile.logo_hint")}
                </p>
              </div>
            ) : null}
          </div>

          <AppForm<NameValues>
            key={profile.updated_at}
            className="flex flex-1 flex-col gap-3 sm:flex-row sm:items-end"
            schema={schema}
            defaultValues={{ name: profile.name }}
            onSubmit={async (values) => {
              await rename.mutateAsync(values);
            }}
          >
            <AppInput
              name="name"
              label={t("tenant_settings.profile.name")}
              description={t("tenant_settings.profile.name_hint", {
                slug: profile.slug,
              })}
              disabled={!canWrite}
              className="flex-1"
            />
            {canWrite ? (
              <Button
                type="submit"
                disabled={rename.isPending}
                className="sm:mb-6"
              >
                {t("tenant_settings.profile.save")}
              </Button>
            ) : null}
          </AppForm>
        </div>
      )}
      <ConfirmDialog
        open={confirmRemove}
        title={t("tenant_settings.profile.logo_remove_title")}
        description={t("tenant_settings.profile.logo_remove_description")}
        confirmLabel={t("tenant_settings.profile.logo_remove")}
        variant="destructive"
        isPending={removeLogo.isPending}
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() => removeLogo.mutate()}
      />
    </EntitySectionCard>
  );
}

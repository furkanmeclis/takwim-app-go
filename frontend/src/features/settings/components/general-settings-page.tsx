"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { permissions } from "@/config/permissions";
import { SettingsForm } from "@/features/io/components/settings-form";
import { ioKeys } from "@/features/io/hooks/query-keys";
import type { PatchSettingsRequest } from "@/features/io/services/settings.service";
import { settingsService } from "@/features/io/services/settings.service";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Branding & letterhead (app_settings singleton) used by exports and public config. */
export function GeneralSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.settings.read);
  const canWrite = can(permissions.settings.write);
  const queryClient = useQueryClient();

  const settingsQuery = useQuery({
    queryKey: ioKeys.settings.all,
    queryFn: () => settingsService.get(),
    enabled: canRead,
  });

  const save = useAppMutation({
    mutationFn: (body: PatchSettingsRequest) => settingsService.patch(body),
    onSuccess: (data) => {
      queryClient.setQueryData(ioKeys.settings.all, data);
      appToast.success(t("settings.toast.saved"));
    },
    onError: () => {
      appToast.error(t("settings.toast.save_failed"));
    },
  });

  const uploadLogo = useAppMutation({
    mutationFn: (file: File) => settingsService.uploadLogo(file),
    onSuccess: (data) => {
      queryClient.setQueryData(ioKeys.settings.all, data);
      appToast.success(t("settings.toast.logo_saved"));
    },
  });

  const removeLogo = useAppMutation({
    mutationFn: () => settingsService.deleteLogo(),
    onSuccess: (data) => {
      queryClient.setQueryData(ioKeys.settings.all, data);
      appToast.success(t("settings.toast.logo_removed"));
    },
  });

  if (!canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("settings.forbidden")}
      />
    );
  }

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.subnav.general_branding")}
        description={t("settings.description")}
      />

      {settingsQuery.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : settingsQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("settings.forbidden")}
          onRetry={() => void settingsQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : settingsQuery.data ? (
        <SettingsForm
          key={JSON.stringify(settingsQuery.data)}
          settings={settingsQuery.data}
          canWrite={canWrite}
          isSaving={save.isPending}
          isLogoUploading={uploadLogo.isPending}
          isLogoRemoving={removeLogo.isPending}
          onSubmit={async (payload) => {
            await save.mutateAsync(payload);
          }}
          onUploadLogo={(file) => uploadLogo.mutate(file)}
          onRemoveLogo={() => removeLogo.mutate()}
        />
      ) : null}
    </div>
  );
}

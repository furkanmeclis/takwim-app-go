import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { systemKeys } from "@/features/settings/hooks/query-keys";
import { systemService } from "@/features/settings/services/system.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Live system diagnostics; disabled for users without `platform.settings.read`. */
export function useSystemInfo(options?: { refetchInterval?: number }) {
  const { can } = usePermission();
  return useQuery({
    queryKey: systemKeys.info(),
    queryFn: () => systemService.getInfo(),
    enabled: can(permissions.settings.read),
    refetchInterval: options?.refetchInterval ?? 30_000,
    staleTime: 10_000,
  });
}

/** Re-runs the dependency probes and replaces the cached snapshot. */
export function useSystemPing() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useMutation({
    mutationFn: () => systemService.ping(),
    onSuccess: (data) => {
      queryClient.setQueryData(systemKeys.info(), data);
      appToast.success(t("settings.system.toast_ping_success"));
    },
    onError: () => {
      appToast.error(t("settings.system.toast_ping_failed"));
    },
  });
}

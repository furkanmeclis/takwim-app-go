import { useMutation, useQueryClient } from "@tanstack/react-query";

import { bulkService } from "@/features/bulk-engine/services/bulk.service";
import type {
  BulkExecuteSyncResult,
  BulkJob,
  BulkResource,
  BulkSummary,
  BulkTarget,
} from "@/features/bulk-engine/types";
import { useApiScope } from "@/features/tenant/hooks/use-api-scope";
import { scopeKey } from "@/lib/api/scoped-request";
import { useLocale } from "@/providers/locale-provider";
import { toast } from "sonner";

function isSyncResult(
  data: BulkJob | BulkExecuteSyncResult,
): data is BulkExecuteSyncResult {
  return "sync" in data && data.sync === true;
}

type ExecuteInput = {
  resource: BulkResource;
  action: string;
  target: BulkTarget;
  onComplete?: () => void;
  /** The summary of a run that finished inline (sync). */
  onResult?: (summary: BulkSummary) => void;
};

export function useBulkMutation() {
  const { locale, t } = useLocale();
  const queryClient = useQueryClient();
  const scope = useApiScope();

  return useMutation({
    mutationFn: async ({ resource, action, target }: ExecuteInput) => {
      return bulkService.execute(
        resource,
        {
          action,
          target,
          locale,
        },
        scope,
      );
    },
    onSuccess: (data, variables) => {
      if (isSyncResult(data)) {
        const { succeeded, failed, skipped = 0 } = data.summary;
        toast.success(
          skipped > 0
            ? t("bulk.result.sync_skipped", { succeeded, failed, skipped })
            : t("bulk.result.sync", { succeeded, failed }),
        );
        variables.onResult?.(data.summary);
        variables.onComplete?.();
        return;
      }
      toast.info(t("bulk.job.queued"));
      void queryClient.invalidateQueries({
        queryKey: ["bulk-jobs", scopeKey(scope)],
      });
      variables.onComplete?.();
    },
    onError: () => {
      toast.error(t("bulk.job.failed"));
    },
  });
}

export function useBulkRollbackMutation() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const scope = useApiScope();

  return useMutation({
    mutationFn: (uuid: string) => bulkService.rollback(uuid, scope),
    onSuccess: () => {
      toast.success(t("bulk.rollback_success"));
      void queryClient.invalidateQueries({
        queryKey: ["bulk-jobs", scopeKey(scope)],
      });
    },
    onError: () => {
      toast.error(t("common.error_generic"));
    },
  });
}

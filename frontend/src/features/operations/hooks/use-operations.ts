"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { operationsService } from "@/features/operations/services/operations.service";
import type {
  ListNoticeFailuresParams,
  ListPlatformIntegrationJobsParams,
  ListQueueTasksParams,
  QueueTask,
} from "@/features/operations/types";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const operationsKeys = {
  all: ["platform", "operations"] as const,
  summary: () => [...operationsKeys.all, "summary"] as const,
  queueTasks: (params: ListQueueTasksParams) =>
    [...operationsKeys.all, "queue-tasks", params] as const,
  integrationJobs: (params: ListPlatformIntegrationJobsParams) =>
    [...operationsKeys.all, "integration-jobs", params] as const,
  integrationJob: (uuid: string) =>
    [...operationsKeys.all, "integration-job", uuid] as const,
  noticeFailures: (params: ListNoticeFailuresParams) =>
    [...operationsKeys.all, "notice-failures", params] as const,
};

/** Queue sizes, workers and failure counts; refreshed every 30 seconds. */
export function useOperationsSummary() {
  const { can } = usePermission();
  return useQuery({
    queryKey: operationsKeys.summary(),
    queryFn: () => operationsService.summary(),
    enabled: can(permissions.settings.read),
    refetchInterval: 30_000,
    staleTime: 10_000,
  });
}

export function useQueueTasks(params: ListQueueTasksParams) {
  return useQuery({
    queryKey: operationsKeys.queueTasks(params),
    queryFn: () => operationsService.queueTasks(params),
    placeholderData: (previous) => previous,
  });
}

export function usePlatformIntegrationJobs(
  params: ListPlatformIntegrationJobsParams,
) {
  return useQuery({
    queryKey: operationsKeys.integrationJobs(params),
    queryFn: () => operationsService.integrationJobs(params),
    placeholderData: (previous) => previous,
  });
}

export function usePlatformIntegrationJob(uuid: string | null) {
  return useQuery({
    queryKey: operationsKeys.integrationJob(uuid ?? ""),
    queryFn: () => operationsService.integrationJob(uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

export function useNoticeFailures(params: ListNoticeFailuresParams) {
  return useQuery({
    queryKey: operationsKeys.noticeFailures(params),
    queryFn: () => operationsService.noticeFailures(params),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateOperations() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: operationsKeys.all });
}

function errorCode(error: unknown) {
  return error instanceof ApiError ? error.code : "";
}

/** Run a failed queue task again now. */
export function useRunQueueTask() {
  const { t } = useLocale();
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (task: QueueTask) => operationsService.runQueueTask(task),
    meta: { silent: true },
    onSuccess: () => appToast.success(t("operations.queue.toast.run")),
    onError: (error) =>
      appToast.error(
        t(
          errorCode(error) === "QUEUE_TASK_NOT_FAILED"
            ? "operations.queue.toast.not_failed"
            : "operations.queue.toast.run_failed",
        ),
      ),
    onSettled: () => void invalidate(),
  });
}

/** Drop a failed queue task. */
export function useDeleteQueueTask() {
  const { t } = useLocale();
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (task: QueueTask) => operationsService.deleteQueueTask(task),
    meta: { silent: true },
    onSuccess: () => appToast.success(t("operations.queue.toast.deleted")),
    onError: (error) =>
      appToast.error(
        t(
          errorCode(error) === "QUEUE_TASK_NOT_FAILED"
            ? "operations.queue.toast.not_failed"
            : "operations.queue.toast.delete_failed",
        ),
      ),
    onSettled: () => void invalidate(),
  });
}

/** The runner's retry: the provider is asked first, only work it confirms was not done is sent again. */
export function useRetryPlatformIntegrationJob() {
  const { t } = useLocale();
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (uuid: string) => operationsService.retryIntegrationJob(uuid),
    meta: { silent: true },
    onSuccess: (result) => {
      const key =
        result.action === "already_done"
          ? "integrations.jobs.toast.already_done"
          : result.action === "in_progress"
            ? "integrations.jobs.toast.in_progress"
            : "integrations.jobs.toast.resubmitted";
      appToast.success(t(key));
    },
    onError: (error) => {
      const code = errorCode(error);
      appToast.error(
        t(
          code === "INTEGRATION_JOB_UNVERIFIED"
            ? "integrations.jobs.toast.unverified"
            : code === "INTEGRATION_JOB_BUSY"
              ? "integrations.jobs.toast.busy"
              : code === "INTEGRATION_JOB_NOT_RETRYABLE"
                ? "integrations.jobs.toast.not_retryable"
                : "integrations.jobs.toast.retry_failed",
        ),
      );
    },
    onSettled: () => void invalidate(),
  });
}

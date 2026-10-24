import type {
  IntegrationJobDetail,
  IntegrationJobRetry,
} from "@/features/integrations/types";
import type {
  ListNoticeFailuresParams,
  ListPlatformIntegrationJobsParams,
  ListQueueTasksParams,
  NoticeFailurePage,
  OperationsSummary,
  PlatformIntegrationJobPage,
  QueueTask,
  QueueTaskPage,
} from "@/features/operations/types";
import { apiClient, unwrap } from "@/lib/api";

/** Platform operations (TEC-930): background work health and failed work. */
export const operationsService = {
  async summary() {
    return unwrap<OperationsSummary>(
      await apiClient.GET("/v1/platform/operations/summary"),
    );
  },

  async queueTasks(params: ListQueueTasksParams) {
    return unwrap<QueueTaskPage>(
      await apiClient.GET("/v1/platform/operations/queue-tasks", {
        params: { query: params },
      }),
    );
  },

  async runQueueTask(task: Pick<QueueTask, "queue" | "id">) {
    return unwrap<QueueTask>(
      await apiClient.POST(
        "/v1/platform/operations/queue-tasks/{queue}/{id}/run",
        { params: { path: { queue: task.queue, id: task.id } } },
      ),
    );
  },

  async deleteQueueTask(task: Pick<QueueTask, "queue" | "id">) {
    return unwrap<QueueTask>(
      await apiClient.DELETE(
        "/v1/platform/operations/queue-tasks/{queue}/{id}",
        { params: { path: { queue: task.queue, id: task.id } } },
      ),
    );
  },

  async integrationJobs(params: ListPlatformIntegrationJobsParams) {
    return unwrap<PlatformIntegrationJobPage>(
      await apiClient.GET("/v1/platform/operations/integration-jobs", {
        params: { query: params },
      }),
    );
  },

  async integrationJob(uuid: string) {
    return unwrap<IntegrationJobDetail>(
      await apiClient.GET("/v1/platform/operations/integration-jobs/{uuid}", {
        params: { path: { uuid } },
      }),
    );
  },

  async retryIntegrationJob(uuid: string) {
    return unwrap<IntegrationJobRetry>(
      await apiClient.POST(
        "/v1/platform/operations/integration-jobs/{uuid}/retry",
        { params: { path: { uuid } } },
      ),
    );
  },

  async noticeFailures(params: ListNoticeFailuresParams) {
    return unwrap<NoticeFailurePage>(
      await apiClient.GET("/v1/platform/operations/notice-failures", {
        params: { query: params },
      }),
    );
  },
};

import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type OperationsSummary = Schemas["OperationsSummary"];
export type OperationsQueueStats = Schemas["OperationsQueueStats"];
export type QueueTask = Schemas["QueueTask"];
export type QueueTaskState = QueueTask["state"];
export type QueueTaskPage = Schemas["EnvelopeQueueTaskPage"]["data"];
export type PlatformIntegrationJob = Schemas["PlatformIntegrationJob"];
export type PlatformIntegrationJobPage =
  Schemas["EnvelopePlatformIntegrationJobPage"]["data"];
export type NoticeFailure = Schemas["NoticeFailure"];
export type NoticeFailurePage = Schemas["EnvelopeNoticeFailurePage"]["data"];

export type ListQueueTasksParams = {
  limit: number;
  offset: number;
  state: QueueTaskState;
  queue?: string;
};

export type ListPlatformIntegrationJobsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
};

export type ListNoticeFailuresParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  channel?: string;
};

import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ClassSchedule = Schemas["ClassSchedule"];
export type ClassScheduleInput = Schemas["ClassScheduleInput"];
export type ClassSchedulePage = Schemas["ClassSchedulePage"];
export type ClassFacets = Schemas["ClassFacets"];
export type ClassFormOptions = Schemas["ClassFormOptions"];
export type ClassFormService = Schemas["ClassFormService"];
export type ClassFormResource = Schemas["ClassFormResource"];
export type ClassSession = Schemas["ClassSession"];
export type ClassSessionPage = Schemas["ClassSessionPage"];
export type ClassSessionDetail = Schemas["ClassSessionDetail"];
export type ClassAttendee = Schemas["ClassAttendee"];
export type ClassSessionChangeInput = Schemas["ClassSessionChangeInput"];
export type ClassSessionCancelInput = Schemas["ClassSessionCancelInput"];
export type ClassSessionCancelResult = Schemas["ClassSessionCancelResult"];
export type ClassSessionCancellation = Schemas["ClassSessionCancellation"];
export type ClassSessionMoveInput = Schemas["ClassSessionMoveInput"];
export type ClassSessionMoveResult = Schemas["ClassSessionMoveResult"];
export type ClassSessionMove = Schemas["ClassSessionMove"];
export type ClassSessionMoveIssue = Schemas["ClassSessionMoveIssue"];
export type BulkJob = Schemas["BulkJob"];

export type Weekday = ClassSchedule["weekday"];
export type ClassSessionState = ClassSession["state"];

/** Query of `GET /v1/tenant/classes`. */
export type ListClassesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  branch?: string;
  service?: string;
  staff?: string;
  weekday?: string;
  is_active?: "true" | "false";
};

/** Query of `GET /v1/tenant/class-sessions` (local dates "2026-06-01"). */
export type ListClassSessionsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  from: string;
  to: string;
  branch?: string;
  service?: string;
  staff?: string;
  state?: string;
};

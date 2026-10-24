import type {
  ClassFacets,
  ClassFormOptions,
  ClassSchedule,
  ClassScheduleInput,
  ClassSchedulePage,
  ClassSessionChangeInput,
  ClassSessionDetail,
  ClassSessionPage,
  ListClassSessionsParams,
  ListClassesParams,
} from "@/features/classes/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const CLASSES = "/v1/tenant/classes";
const SESSIONS = "/v1/tenant/class-sessions";

/**
 * Group class timetable (TEC-970). A class is a group service taught on
 * one weekday at one time in a branch; each such day is a session, the
 * only times the class can be booked. Writes are `silent`: the feature
 * shows their errors in its own words.
 */
export const classesService = {
  list(slug: string, params: ListClassesParams) {
    return tenantRequest<ClassSchedulePage>(slug, "GET", CLASSES, {
      query: params,
    });
  },

  facets(slug: string) {
    return tenantRequest<ClassFacets>(slug, "GET", `${CLASSES}/facets`);
  },

  form(slug: string) {
    return tenantRequest<ClassFormOptions>(slug, "GET", `${CLASSES}/form`);
  },

  create(slug: string, body: ClassScheduleInput) {
    return tenantRequest<ClassSchedule>(slug, "POST", CLASSES, {
      body,
      silent: true,
    });
  },

  update(slug: string, uuid: string, body: ClassScheduleInput) {
    return tenantRequest<ClassSchedule>(slug, "PATCH", `${CLASSES}/${uuid}`, {
      body,
      silent: true,
    });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", `${CLASSES}/${uuid}`, {
      silent: true,
    });
  },

  sessions(slug: string, params: ListClassSessionsParams) {
    return tenantRequest<ClassSessionPage>(slug, "GET", SESSIONS, {
      query: params,
    });
  },

  session(slug: string, uuid: string, date: string) {
    return tenantRequest<ClassSessionDetail>(
      slug,
      "GET",
      `${CLASSES}/${uuid}/sessions/${date}`,
    );
  },

  changeSession(
    slug: string,
    uuid: string,
    date: string,
    body: ClassSessionChangeInput,
  ) {
    return tenantRequest<ClassSessionDetail>(
      slug,
      "PATCH",
      `${CLASSES}/${uuid}/sessions/${date}`,
      { body, silent: true },
    );
  },
};

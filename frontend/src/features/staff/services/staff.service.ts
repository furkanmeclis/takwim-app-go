import type { ResourceMeta } from "@/features/io/types";
import type {
  ListStaffParams,
  Staff,
  StaffAccount,
  StaffActivityPage,
  StaffCreateRequest,
  StaffFacets,
  StaffPage,
  StaffPatchRequest,
  TimeOff,
  TimeOffCreateRequest,
  TimeOffPage,
  TimeOffPatchRequest,
  WeeklyShifts,
} from "@/features/staff/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/staff";

/**
 * Staff of one organization. Every call names the organization through
 * `tenantRequest` (`X-Organization: {slug}`); the API scopes all reads and
 * writes to it and to the member's branches.
 */
export const staffService = {
  list(slug: string, params: ListStaffParams) {
    return tenantRequest<StaffPage>(slug, "GET", BASE, { query: params });
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  facets(slug: string) {
    return tenantRequest<StaffFacets>(slug, "GET", `${BASE}/facets`);
  },

  linkableMembers(slug: string) {
    return tenantRequest<{ items: StaffAccount[] }>(
      slug,
      "GET",
      `${BASE}/linkable-members`,
    );
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Staff>(slug, "GET", `${BASE}/${uuid}`);
  },

  create(slug: string, body: StaffCreateRequest) {
    return tenantRequest<Staff>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: StaffPatchRequest) {
    return tenantRequest<Staff>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },

  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<StaffActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },

  /** Errors are rendered next to the shifts, so no global toast. */
  setShifts(
    slug: string,
    uuid: string,
    branchUuid: string,
    body: WeeklyShifts,
  ) {
    return tenantRequest<Staff>(
      slug,
      "PUT",
      `${BASE}/${uuid}/branches/${branchUuid}/shifts`,
      { body, silent: true },
    );
  },

  listTimeOff(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<TimeOffPage>(slug, "GET", `${BASE}/${uuid}/time-off`, {
      query: params,
    });
  },

  createTimeOff(slug: string, uuid: string, body: TimeOffCreateRequest) {
    return tenantRequest<TimeOff>(slug, "POST", `${BASE}/${uuid}/time-off`, {
      body,
    });
  },

  updateTimeOff(
    slug: string,
    uuid: string,
    timeOffUuid: string,
    body: TimeOffPatchRequest,
  ) {
    return tenantRequest<TimeOff>(
      slug,
      "PATCH",
      `${BASE}/${uuid}/time-off/${timeOffUuid}`,
      { body },
    );
  },

  removeTimeOff(slug: string, uuid: string, timeOffUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/time-off/${timeOffUuid}`,
    );
  },
};

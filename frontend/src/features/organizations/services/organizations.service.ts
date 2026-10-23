import type { ResourceMeta } from "@/features/io/types";
import type {
  BranchOption,
  CreateOrganizationRequest,
  CreateOrganizationResult,
  ListOrganizationsParams,
  Organization,
  OrganizationActivityPage,
  OrganizationSummary,
  OrganizationSummaryPage,
  PatchOrganizationRequest,
  PlatformMemberPatchRequest,
  PlatformMemberRequest,
} from "@/features/organizations/types";
import type {
  ListTenantMembersParams,
  TenantMember,
  TenantMemberPage,
} from "@/features/tenant-members/types";
import type { TenantRole } from "@/features/tenant-roles/types";
import { platformRequest } from "@/lib/api/platform-request";

const BASE = "/v1/platform/organizations";

/** Platform organization management (`platform.organizations.*`). */
export const organizationsService = {
  list(params: ListOrganizationsParams) {
    return platformRequest<OrganizationSummaryPage>("GET", BASE, {
      query: params,
    });
  },

  meta() {
    return platformRequest<ResourceMeta>("GET", `${BASE}/meta`);
  },

  get(uuid: string) {
    return platformRequest<OrganizationSummary>("GET", `${BASE}/${uuid}`);
  },

  create(body: CreateOrganizationRequest) {
    return platformRequest<CreateOrganizationResult>("POST", BASE, { body });
  },

  update(uuid: string, body: PatchOrganizationRequest) {
    return platformRequest<Organization>("PATCH", `${BASE}/${uuid}`, { body });
  },

  remove(uuid: string) {
    return platformRequest<{ status: string }>("DELETE", `${BASE}/${uuid}`);
  },

  setStatus(uuid: string, status: "active" | "suspended") {
    return platformRequest<Organization>("POST", `${BASE}/${uuid}/status`, {
      body: { status },
    });
  },

  setAccess(
    uuid: string,
    body: { access_starts_at?: string; access_ends_at?: string | null },
  ) {
    return platformRequest<Organization>("POST", `${BASE}/${uuid}/access`, {
      body,
    });
  },

  activity(uuid: string, params: { limit: number; offset: number }) {
    return platformRequest<OrganizationActivityPage>(
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },

  roles(uuid: string) {
    return platformRequest<{ items: TenantRole[] }>(
      "GET",
      `${BASE}/${uuid}/roles`,
    );
  },

  branches(uuid: string) {
    return platformRequest<{ items: BranchOption[] }>(
      "GET",
      `${BASE}/${uuid}/branches`,
    );
  },

  members(uuid: string, params: ListTenantMembersParams) {
    return platformRequest<TenantMemberPage>("GET", `${BASE}/${uuid}/members`, {
      query: params,
    });
  },

  addMember(uuid: string, body: PlatformMemberRequest) {
    return platformRequest<TenantMember>("POST", `${BASE}/${uuid}/members`, {
      body,
    });
  },

  updateMember(
    uuid: string,
    userUuid: string,
    body: PlatformMemberPatchRequest,
  ) {
    return platformRequest<TenantMember>(
      "PATCH",
      `${BASE}/${uuid}/members/${userUuid}`,
      { body },
    );
  },

  removeMember(uuid: string, userUuid: string) {
    return platformRequest<{ status: string }>(
      "DELETE",
      `${BASE}/${uuid}/members/${userUuid}`,
    );
  },
};

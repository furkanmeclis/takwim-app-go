"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { organizationsService } from "@/features/organizations/services/organizations.service";
import type {
  CreateOrganizationRequest,
  ListOrganizationsParams,
  PatchOrganizationRequest,
  PlatformMemberPatchRequest,
  PlatformMemberRequest,
} from "@/features/organizations/types";
import type { ListTenantMembersParams } from "@/features/tenant-members/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const organizationKeys = {
  all: ["platform", "organizations"] as const,
  list: (params: ListOrganizationsParams) =>
    [...organizationKeys.all, "list", params] as const,
  meta: () => [...organizationKeys.all, "meta"] as const,
  detail: (uuid: string) => [...organizationKeys.all, uuid] as const,
  members: (uuid: string, params: ListTenantMembersParams) =>
    [...organizationKeys.detail(uuid), "members", params] as const,
  roles: (uuid: string) => [...organizationKeys.detail(uuid), "roles"] as const,
  branches: (uuid: string) =>
    [...organizationKeys.detail(uuid), "branches"] as const,
  activity: (uuid: string, limit: number) =>
    [...organizationKeys.detail(uuid), "activity", limit] as const,
  overview: (uuid: string) =>
    [...organizationKeys.detail(uuid), "overview"] as const,
};

export function useOrganizationsList(params: ListOrganizationsParams) {
  return useQuery({
    queryKey: organizationKeys.list(params),
    queryFn: () => organizationsService.list(params),
    placeholderData: (previous) => previous,
  });
}

export function useOrganizationsMeta() {
  return useQuery({
    queryKey: organizationKeys.meta(),
    queryFn: () => organizationsService.meta(),
    staleTime: 5 * 60_000,
  });
}

export function useOrganization(uuid: string) {
  return useQuery({
    queryKey: organizationKeys.detail(uuid),
    queryFn: () => organizationsService.get(uuid),
  });
}

/** The 360 overview of one organization (TEC-927). */
export function useOrganizationOverview(uuid: string) {
  return useQuery({
    queryKey: organizationKeys.overview(uuid),
    queryFn: () => organizationsService.overview(uuid),
    staleTime: 30_000,
  });
}

export function useOrganizationRoles(uuid: string, enabled = true) {
  return useQuery({
    queryKey: organizationKeys.roles(uuid),
    queryFn: () => organizationsService.roles(uuid),
    enabled,
    staleTime: 60_000,
  });
}

export function useOrganizationBranches(uuid: string, enabled = true) {
  return useQuery({
    queryKey: organizationKeys.branches(uuid),
    queryFn: () => organizationsService.branches(uuid),
    enabled,
    staleTime: 60_000,
  });
}

export function useOrganizationMembers(
  uuid: string,
  params: ListTenantMembersParams,
  enabled = true,
) {
  return useQuery({
    queryKey: organizationKeys.members(uuid, params),
    queryFn: () => organizationsService.members(uuid, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useOrganizationActivity(uuid: string, limit: number) {
  return useQuery({
    queryKey: organizationKeys.activity(uuid, limit),
    queryFn: () => organizationsService.activity(uuid, { limit, offset: 0 }),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateOrganizations() {
  const queryClient = useQueryClient();
  return () =>
    void queryClient.invalidateQueries({ queryKey: organizationKeys.all });
}

export function useCreateOrganization() {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: (body: CreateOrganizationRequest) =>
      organizationsService.create(body),
    onSuccess: (result) => {
      appToast.success(
        result.owner_invitation
          ? t("organizations.toast.created_invited", {
              email: result.owner_invitation.email,
            })
          : t("organizations.toast.created"),
      );
      invalidate();
    },
  });
}

export function useUpdateOrganization() {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PatchOrganizationRequest;
    }) => organizationsService.update(uuid, body),
    onSuccess: () => {
      appToast.success(t("organizations.toast.updated"));
      invalidate();
    },
  });
}

export function useDeleteOrganization() {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: (uuid: string) => organizationsService.remove(uuid),
    onSuccess: () => {
      appToast.success(t("organizations.toast.deleted"));
      invalidate();
    },
  });
}

export function useSetOrganizationStatus() {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: ({
      uuid,
      status,
    }: {
      uuid: string;
      status: "active" | "suspended";
    }) => organizationsService.setStatus(uuid, status),
    onSuccess: (org) => {
      appToast.success(
        org.status === "active"
          ? t("organizations.toast.activated")
          : t("organizations.toast.suspended"),
      );
      invalidate();
    },
  });
}

export function useSetOrganizationAccess() {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: { access_starts_at?: string; access_ends_at?: string | null };
    }) => organizationsService.setAccess(uuid, body),
    onSuccess: () => {
      appToast.success(t("organizations.toast.access_updated"));
      invalidate();
    },
  });
}

export function useAddOrganizationMember(uuid: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: (body: PlatformMemberRequest) =>
      organizationsService.addMember(uuid, body),
    onSuccess: () => {
      appToast.success(t("organizations.members.toast.added"));
      invalidate();
    },
  });
}

export function useUpdateOrganizationMember(uuid: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: ({
      userUuid,
      body,
    }: {
      userUuid: string;
      body: PlatformMemberPatchRequest;
    }) => organizationsService.updateMember(uuid, userUuid, body),
    onSuccess: () => {
      appToast.success(t("organizations.members.toast.updated"));
      invalidate();
    },
  });
}

export function useRemoveOrganizationMember(uuid: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateOrganizations();
  return useMutation({
    mutationFn: (userUuid: string) =>
      organizationsService.removeMember(uuid, userUuid),
    onSuccess: () => {
      appToast.success(t("organizations.members.toast.removed"));
      invalidate();
    },
  });
}

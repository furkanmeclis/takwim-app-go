"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { staffKeys } from "@/features/staff/hooks/query-keys";
import { staffService } from "@/features/staff/services/staff.service";
import type {
  ListStaffParams,
  StaffCreateRequest,
  StaffPatchRequest,
  TimeOffCreateRequest,
  TimeOffPatchRequest,
  WeeklyShifts,
} from "@/features/staff/types";
import type { BranchPage } from "@/features/branches/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useStaffList(params: ListStaffParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffKeys.list(slug, params),
    queryFn: () => staffService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useStaffMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffKeys.meta(slug),
    queryFn: () => staffService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useStaffFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffKeys.facets(slug),
    queryFn: () => staffService.facets(slug),
    staleTime: 60_000,
  });
}

export function useLinkableMembers(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffKeys.linkable(slug),
    queryFn: () => staffService.linkableMembers(slug),
    enabled,
  });
}

/**
 * Every branch the member may access (same cache as the header branch
 * picker), for the branch choice of the staff form.
 */
export function useAccessibleBranches() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: ["tenant", slug, "branch-picker"],
    queryFn: () =>
      tenantRequest<BranchPage>(slug, "GET", "/v1/tenant/branches", {
        query: { limit: 100, sort: "name" },
        branch: false,
        silent: true,
      }),
    staleTime: 60_000,
  });
}

export function useStaffMember(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: staffKeys.detail(slug, uuid ?? ""),
    queryFn: () => staffService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useStaffTimeOff(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...staffKeys.timeOff(slug, uuid), params],
    queryFn: () => staffService.listTimeOff(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useStaffActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...staffKeys.activity(slug, uuid), params],
    queryFn: () => staffService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

function useInvalidateStaff() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: staffKeys.all(slug) });
}

export function useCreateStaff() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaff();
  return useMutation({
    mutationFn: (body: StaffCreateRequest) => staffService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("staff.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateStaff() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaff();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: StaffPatchRequest }) =>
      staffService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("staff.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteStaff() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaff();
  return useMutation({
    mutationFn: (uuid: string) => staffService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("staff.toast.deleted"));
      void invalidate();
    },
  });
}

export function useSetShifts(staffUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaff();
  return useMutation({
    mutationFn: ({
      branchUuid,
      shifts,
    }: {
      branchUuid: string;
      shifts: WeeklyShifts;
    }) => staffService.setShifts(slug, staffUuid, branchUuid, shifts),
    onSuccess: () => {
      appToast.success(t("staff.shifts.toast.saved"));
      void invalidate();
    },
  });
}

export function useSaveTimeOff(staffUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaff();
  return useMutation({
    mutationFn: ({
      timeOffUuid,
      body,
    }: {
      timeOffUuid?: string;
      body: TimeOffCreateRequest | TimeOffPatchRequest;
    }) =>
      timeOffUuid
        ? staffService.updateTimeOff(slug, staffUuid, timeOffUuid, body)
        : staffService.createTimeOff(
            slug,
            staffUuid,
            body as TimeOffCreateRequest,
          ),
    onSuccess: (_data, variables) => {
      appToast.success(
        t(
          variables.timeOffUuid
            ? "staff.time_off.toast.updated"
            : "staff.time_off.toast.created",
        ),
      );
      void invalidate();
    },
  });
}

export function useDeleteTimeOff(staffUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateStaff();
  return useMutation({
    mutationFn: (timeOffUuid: string) =>
      staffService.removeTimeOff(slug, staffUuid, timeOffUuid),
    onSuccess: () => {
      appToast.success(t("staff.time_off.toast.deleted"));
      void invalidate();
    },
  });
}

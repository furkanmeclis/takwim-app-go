"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { invitationsService } from "@/features/invitations/services/invitations.service";
import type {
  CreateInvitationRequest,
  InvitationScope,
  ListInvitationsParams,
} from "@/features/invitations/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function scopeKey(scope: InvitationScope) {
  return scope.kind === "tenant"
    ? (["tenant", scope.slug] as const)
    : (["platform", "organizations", scope.organizationUuid] as const);
}

/** Query keys carry the organization (slug or uuid). */
export const invitationKeys = {
  all: (scope: InvitationScope) => [...scopeKey(scope), "invitations"] as const,
  list: (scope: InvitationScope, params: ListInvitationsParams) =>
    [...invitationKeys.all(scope), "list", params] as const,
};

export function useInvitationsList(
  scope: InvitationScope,
  params: ListInvitationsParams,
  enabled = true,
) {
  return useQuery({
    queryKey: invitationKeys.list(scope, params),
    queryFn: () => invitationsService.list(scope, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

function useInvalidate(scope: InvitationScope) {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: invitationKeys.all(scope) });
    if (scope.kind === "platform") {
      // Pending invitation count and owner on the organization pages.
      void queryClient.invalidateQueries({
        queryKey: ["platform", "organizations"],
      });
    }
  };
}

export function useCreateInvitation(scope: InvitationScope) {
  const { t } = useLocale();
  const invalidate = useInvalidate(scope);
  return useMutation({
    mutationFn: (body: CreateInvitationRequest) =>
      invitationsService.create(scope, body),
    onSuccess: (invitation) => {
      appToast.success(
        t("invitations.toast.created", { email: invitation.email }),
      );
      invalidate();
    },
  });
}

export function useResendInvitation(scope: InvitationScope) {
  const { t } = useLocale();
  const invalidate = useInvalidate(scope);
  return useMutation({
    mutationFn: (uuid: string) => invitationsService.resend(scope, uuid),
    onSuccess: (invitation) => {
      appToast.success(
        t("invitations.toast.resent", { email: invitation.email }),
      );
      invalidate();
    },
  });
}

export function useRevokeInvitation(scope: InvitationScope) {
  const { t } = useLocale();
  const invalidate = useInvalidate(scope);
  return useMutation({
    mutationFn: (uuid: string) => invitationsService.revoke(scope, uuid),
    onSuccess: () => {
      appToast.success(t("invitations.toast.revoked"));
      invalidate();
    },
  });
}

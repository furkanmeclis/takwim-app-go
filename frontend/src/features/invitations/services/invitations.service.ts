import type {
  AcceptInvitationResult,
  CreateInvitationRequest,
  Invitation,
  InvitationPage,
  InvitationPreview,
  InvitationScope,
  ListInvitationsParams,
  RegisterInvitedRequest,
} from "@/features/invitations/types";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";

function request<T>(
  scope: InvitationScope,
  method: string,
  suffix: string,
  options?: {
    query?: Record<string, string | number | undefined>;
    body?: unknown;
  },
) {
  if (scope.kind === "tenant") {
    return tenantRequest<T>(
      scope.slug,
      method,
      `/v1/tenant/invitations${suffix}`,
      options,
    );
  }
  return platformRequest<T>(
    method,
    `/v1/platform/organizations/${scope.organizationUuid}/invitations${suffix}`,
    options,
  );
}

/** Invitation management (tenant shell or platform operator). */
export const invitationsService = {
  list(scope: InvitationScope, params: ListInvitationsParams) {
    return request<InvitationPage>(scope, "GET", "", { query: params });
  },

  create(scope: InvitationScope, body: CreateInvitationRequest) {
    return request<Invitation>(scope, "POST", "", { body });
  },

  revoke(scope: InvitationScope, uuid: string) {
    return request<Invitation>(scope, "POST", `/${uuid}/revoke`);
  },

  resend(scope: InvitationScope, uuid: string) {
    return request<Invitation>(scope, "POST", `/${uuid}/resend`);
  },
};

/** Public accept flow of `/invite/{token}` (errors rendered by the page). */
export const invitationAcceptService = {
  preview(token: string) {
    return platformRequest<InvitationPreview>(
      "GET",
      `/v1/invitations/${encodeURIComponent(token)}`,
      { silent: true },
    );
  },

  accept(token: string) {
    return platformRequest<AcceptInvitationResult>(
      "POST",
      `/v1/invitations/${encodeURIComponent(token)}/accept`,
      { silent: true },
    );
  },

  register(token: string, body: RegisterInvitedRequest) {
    return platformRequest<AcceptInvitationResult>(
      "POST",
      `/v1/invitations/${encodeURIComponent(token)}/register`,
      { body, silent: true },
    );
  },
};

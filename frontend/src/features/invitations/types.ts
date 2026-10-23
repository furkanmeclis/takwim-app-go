import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Invitation = Schemas["Invitation"];
export type InvitationPage = Schemas["InvitationPage"];
export type InvitationStatus = Invitation["status"];
export type CreateInvitationRequest = Schemas["CreateInvitationRequest"];
export type InvitationPreview = Schemas["InvitationPreview"];
export type AcceptInvitationResult = Schemas["AcceptInvitationResult"];
export type RegisterInvitedRequest = Schemas["RegisterInvitedRequest"];

/**
 * Where invitations are managed: the tenant shell (`/v1/tenant/invitations`,
 * `X-Organization`) or a platform operator on an organization's detail page
 * (`/v1/platform/organizations/{uuid}/invitations`).
 */
export type InvitationScope =
  | { kind: "tenant"; slug: string }
  | { kind: "platform"; organizationUuid: string };

/** List query of the invitation endpoints. */
export type ListInvitationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated statuses. */
  status?: string;
};

export const INVITATION_STATUSES = [
  "pending",
  "expired",
  "accepted",
  "revoked",
] as const satisfies readonly InvitationStatus[];

/** Engine slug (live list channel `org:{uuid}:resource:tenant.invitations`). */
export const TENANT_INVITATIONS_RESOURCE = "tenant.invitations" as const;

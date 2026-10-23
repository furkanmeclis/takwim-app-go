import type { ListRealtimeOptions } from "@/components/tables";

/**
 * Live list channel of an organization-owned resource
 * (`org:{org_uuid}:resource:{name}`, see backend `tenant.ResourceChannel`).
 * The subscription token is only issued to members whose tenant role grants
 * the resource's read permission.
 */
export function tenantResourceChannel(orgUuid: string, resource: string) {
  return `org:${orgUuid}:resource:${resource}`;
}

/** `useServerListState` realtime option for a tenant list; off until the organization is known. */
export function tenantLiveList(
  organization: { uuid: string } | null | undefined,
  resource: string,
): ListRealtimeOptions | undefined {
  if (!organization?.uuid) return undefined;
  return {
    resource,
    channel: tenantResourceChannel(organization.uuid, resource),
  };
}

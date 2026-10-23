"use client";

import { useMemo, useState } from "react";

import type { MultiComboboxOption } from "@/components/forms";
import { permissions } from "@/config/permissions";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { InvitationsTable } from "@/features/invitations/components/invitations-table";
import { InviteDrawer } from "@/features/invitations/components/invite-drawer";
import {
  TENANT_INVITATIONS_RESOURCE,
  type InvitationScope,
} from "@/features/invitations/types";
import { useAllTenantRoles } from "@/features/tenant-roles/hooks/use-tenant-roles";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const BRANCH_OPTIONS_PARAMS = { limit: 100, offset: 0, sort: "code" };

/** "Davetler" tab of the members page (`/v1/tenant/invitations`). */
export function TenantInvitationsTab({ active }: { active: boolean }) {
  const { t } = useLocale();
  const { slug, organization, branchScope } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.members.write);
  const [inviting, setInviting] = useState(false);
  const scope = useMemo<InvitationScope>(
    () => ({ kind: "tenant", slug }),
    [slug],
  );
  const rolesQuery = useAllTenantRoles(inviting);
  const branchesQuery = useBranchesList(BRANCH_OPTIONS_PARAMS);

  const roleOptions = useMemo<MultiComboboxOption[]>(
    () =>
      (rolesQuery.data?.items ?? []).map((role) => ({
        value: role.uuid,
        label: tenantRoleLabel(t, role),
        hint: role.is_template
          ? t("tenant_roles.template")
          : t("tenant_roles.custom"),
      })),
    [rolesQuery.data?.items, t],
  );
  const branchOptions = useMemo<MultiComboboxOption[]>(
    () =>
      (branchesQuery.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.code} · ${branch.name}`,
        hint: branch.city || undefined,
      })),
    [branchesQuery.data?.items],
  );

  return (
    <>
      <InvitationsTable
        scope={scope}
        canWrite={canWrite}
        onInvite={() => setInviting(true)}
        realtime={tenantLiveList(organization, TENANT_INVITATIONS_RESOURCE)}
        persistKey="tenant-invitations-v1"
        enabled={active}
      />
      <InviteDrawer
        open={inviting}
        onOpenChange={setInviting}
        scope={scope}
        roleOptions={roleOptions}
        branchOptions={branchOptions}
        restricted={branchScope === "selected"}
      />
    </>
  );
}

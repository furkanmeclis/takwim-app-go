"use client";

import { UserPlus } from "lucide-react";
import { useMemo, useState } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityDrawer,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import {
  AppForm,
  AppMultiCombobox,
  AppRadioGroup,
  type MultiComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { InvitationsTable } from "@/features/invitations/components/invitations-table";
import { InviteDrawer } from "@/features/invitations/components/invite-drawer";
import type { InvitationScope } from "@/features/invitations/types";
import {
  useAddOrganizationMember,
  useOrganizationBranches,
  useOrganizationMembers,
  useOrganizationRoles,
  useRemoveOrganizationMember,
  useUpdateOrganizationMember,
} from "@/features/organizations/hooks/use-organizations";
import type { MemberRowActionHandlers } from "@/features/tenant-members/components/member-row-actions";
import { useMembersColumns } from "@/features/tenant-members/components/members-columns";
import {
  createMemberAccessSchema,
  memberAccessDefaults,
  memberAccessRequest,
} from "@/features/tenant-members/schemas/member-access";
import type {
  ListTenantMembersParams,
  TenantMember,
} from "@/features/tenant-members/types";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "platform-member-form";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function createAddSchema(t: (key: string) => string) {
  return createMemberAccessSchema(t).and(
    z.object({
      user_uuid: z
        .string()
        .min(1, t("organizations.members.validation.user_required")),
    }),
  );
}

type MemberFormValues = {
  user_uuid?: string;
  role_uuids: string[];
  branch_scope: "all" | "selected";
  branch_uuids: string[];
};

function BranchPicker({ options }: { options: MultiComboboxOption[] }) {
  const { t } = useLocale();
  const scope = useWatch<MemberFormValues, "branch_scope">({
    name: "branch_scope",
  });
  if (scope !== "selected") return null;
  return (
    <AppMultiCombobox
      name="branch_uuids"
      label={t("tenant_members.fields.branches")}
      description={t("tenant_members.fields.branches_hint")}
      options={options}
      placeholder={t("tenant_members.fields.branches_placeholder")}
      emptyText={t("tenant_members.fields.branches_empty")}
    />
  );
}

/**
 * Add an existing user (mode "add") or change a member's roles and branch
 * access (mode "edit") as a platform operator.
 */
function PlatformMemberDrawer({
  organizationUuid,
  mode,
  member,
  open,
  onOpenChange,
  roleOptions,
  branchOptions,
}: {
  organizationUuid: string;
  mode: "add" | "edit";
  member: TenantMember | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roleOptions: MultiComboboxOption[];
  branchOptions: MultiComboboxOption[];
}) {
  const { t } = useLocale();
  const add = useAddOrganizationMember(organizationUuid);
  const update = useUpdateOrganizationMember(organizationUuid);
  const schema = useMemo(
    () => (mode === "add" ? createAddSchema(t) : createMemberAccessSchema(t)),
    [mode, t],
  );
  const pending = add.isPending || update.isPending;
  const defaults: MemberFormValues =
    mode === "edit" && member
      ? memberAccessDefaults(member)
      : {
          user_uuid: "",
          role_uuids: [],
          branch_scope: "all",
          branch_uuids: [],
        };
  const name = member
    ? `${member.user.name} ${member.user.surname}`.trim() || member.user.email
    : "";

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        mode === "add"
          ? t("organizations.members.add_title")
          : t("tenant_members.access.title")
      }
      description={
        mode === "add"
          ? t("organizations.members.add_description")
          : member
            ? t("tenant_members.access.description", {
                name,
                email: member.user.email,
              })
            : undefined
      }
      size="md"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={pending}>
            {mode === "add"
              ? t("organizations.members.add")
              : t("tenant_members.actions.save")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<MemberFormValues>
          key={member?.user.uuid ?? "new"}
          id={FORM_ID}
          className="space-y-5 px-4"
          schema={schema as z.ZodType<MemberFormValues>}
          defaultValues={defaults}
          onSubmit={async (values) => {
            const access = memberAccessRequest({
              role_uuids: values.role_uuids,
              branch_scope: values.branch_scope,
              branch_uuids: values.branch_uuids,
            });
            if (mode === "add") {
              await add.mutateAsync({
                user_uuid: values.user_uuid ?? "",
                ...access,
              });
            } else if (member) {
              await update.mutateAsync({
                userUuid: member.user.uuid,
                body: access,
              });
            }
            onOpenChange(false);
          }}
        >
          {mode === "add" ? (
            <UserAsyncPicker
              name="user_uuid"
              label={t("organizations.members.user")}
              description={t("organizations.members.user_hint")}
            />
          ) : null}
          <AppMultiCombobox
            name="role_uuids"
            label={t("tenant_members.fields.roles")}
            description={t("tenant_members.fields.roles_hint")}
            options={roleOptions}
            placeholder={t("tenant_members.fields.roles_placeholder")}
            emptyText={t("tenant_members.fields.roles_empty")}
          />
          <AppRadioGroup
            name="branch_scope"
            label={t("tenant_members.fields.branch_scope")}
            options={[
              {
                value: "all",
                label: t("tenant_members.branch_scope.all_long"),
              },
              {
                value: "selected",
                label: t("tenant_members.branch_scope.selected_long"),
              },
            ]}
          />
          <BranchPicker options={branchOptions} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

/**
 * "Üyeler" tab of the platform organization page: members DataTable (roles,
 * branch access, status, last login) with add / change access / remove, and
 * the organization's invitations below. The last owner cannot be removed or
 * demoted (`409 LAST_ORGANIZATION_OWNER`, shown as a toast).
 */
export function OrganizationMembersTab({
  organizationUuid,
}: {
  organizationUuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.organizations.write);
  const listState = useServerListState({
    initialSort: "created_at",
    initialPageSize: 20,
  });
  const [drawer, setDrawer] = useState<
    { mode: "add" } | { mode: "edit"; member: TenantMember } | null
  >(null);
  const [removing, setRemoving] = useState<TenantMember | null>(null);
  const [inviting, setInviting] = useState(false);
  const rolesQuery = useOrganizationRoles(organizationUuid);
  const branchesQuery = useOrganizationBranches(organizationUuid);
  const remove = useRemoveOrganizationMember(organizationUuid);

  const listParams = useMemo<ListTenantMembersParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const roles = column("roles");
    const scope = column("branch_scope");
    return {
      ...listState.params,
      role: roles.length ? roles.join(",") : undefined,
      branch_scope:
        scope.length === 1 ? (scope[0] as "all" | "selected") : undefined,
    };
  }, [listState.columnFilters, listState.params]);
  const listQuery = useOrganizationMembers(organizationUuid, listParams);

  const roles = useMemo(() => rolesQuery.data?.items ?? [], [rolesQuery.data]);
  const roleOptions = useMemo<MultiComboboxOption[]>(
    () =>
      roles.map((role) => ({
        value: role.uuid,
        label: tenantRoleLabel(t, role),
        hint: role.is_template
          ? t("tenant_roles.template")
          : t("tenant_roles.custom"),
      })),
    [roles, t],
  );
  const branchOptions = useMemo<MultiComboboxOption[]>(
    () =>
      (branchesQuery.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.code} · ${branch.name}`,
      })),
    [branchesQuery.data?.items],
  );

  const handlers = useMemo<MemberRowActionHandlers>(
    () => ({
      onEditAccess: (member) => setDrawer({ mode: "edit", member }),
      onRemove: setRemoving,
    }),
    [],
  );
  const columns = useMembersColumns({
    handlers,
    roles,
    writePermission: permissions.organizations.write,
  });
  const invitationScope = useMemo<InvitationScope>(
    () => ({ kind: "platform", organizationUuid }),
    [organizationUuid],
  );
  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const removingName = removing
    ? `${removing.user.name} ${removing.user.surname}`.trim() ||
      removing.user.email
    : "";

  return (
    <div className="space-y-6">
      <EntitySectionCard title={t("organizations.tabs.members")} badge={total}>
        <EntityTable
          columns={columns}
          data={listQuery.data?.items ?? []}
          getRowId={(row) => row.user.uuid}
          onRowClick={(member) => {
            if (canWrite) setDrawer({ mode: "edit", member });
          }}
          isLoading={listQuery.isLoading}
          isError={listQuery.isError}
          errorDescription={t("tenant_members.load_failed")}
          onRetry={() => void listQuery.refetch()}
          emptyTitle={t("organizations.members.empty_title")}
          emptyDescription={t("organizations.members.empty_description")}
          pageCount={pageCount}
          state={listState.tableState}
          features={{ persistKey: "platform-organization-members-v1" }}
          toolbarExtra={
            <>
              {canWrite ? (
                <Button
                  type="button"
                  size="sm"
                  onClick={() => setDrawer({ mode: "add" })}
                >
                  <UserPlus className="size-4" />
                  {t("organizations.members.add")}
                </Button>
              ) : null}
              <EntityToolbar
                onRefresh={() => void listQuery.refetch()}
                refreshDisabled={listQuery.isFetching}
              />
            </>
          }
        />
      </EntitySectionCard>

      <EntitySectionCard title={t("tenant_members.tabs.invitations")}>
        <InvitationsTable
          scope={invitationScope}
          canWrite={canWrite}
          onInvite={() => setInviting(true)}
          persistKey="platform-organization-invitations-v1"
        />
      </EntitySectionCard>

      <PlatformMemberDrawer
        organizationUuid={organizationUuid}
        mode={drawer?.mode ?? "add"}
        member={drawer?.mode === "edit" ? drawer.member : null}
        open={Boolean(drawer)}
        onOpenChange={(open) => {
          if (!open) setDrawer(null);
        }}
        roleOptions={roleOptions}
        branchOptions={branchOptions}
      />
      <InviteDrawer
        open={inviting}
        onOpenChange={setInviting}
        scope={invitationScope}
        roleOptions={roleOptions}
        branchOptions={branchOptions}
      />
      <ConfirmDialog
        open={Boolean(removing)}
        title={t("tenant_members.remove.title", { name: removingName })}
        description={t("tenant_members.remove.description", {
          name: removingName,
        })}
        confirmLabel={t("tenant_members.actions.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() => {
          if (!removing) return;
          remove.mutate(removing.user.uuid, {
            onSettled: () => setRemoving(null),
          });
        }}
      />
    </div>
  );
}

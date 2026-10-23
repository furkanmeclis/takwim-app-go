"use client";

import { useMemo } from "react";
import { useWatch } from "react-hook-form";

import { EntityDrawer } from "@/components/entity";
import {
  AppForm,
  AppMultiCombobox,
  AppRadioGroup,
  type MultiComboboxOption,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useUpdateTenantMember } from "@/features/tenant-members/hooks/use-tenant-members";
import {
  createMemberAccessSchema,
  memberAccessDefaults,
  memberAccessRequest,
  type MemberAccessValues,
} from "@/features/tenant-members/schemas/member-access";
import type { TenantMember } from "@/features/tenant-members/types";
import { useAllTenantRoles } from "@/features/tenant-roles/hooks/use-tenant-roles";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "tenant-member-access";
const BRANCH_OPTIONS_PARAMS = { limit: 100, offset: 0, sort: "code" };

function BranchPicker({
  branchOptions,
}: {
  branchOptions: MultiComboboxOption[];
}) {
  const { t } = useLocale();
  const scope = useWatch<MemberAccessValues, "branch_scope">({
    name: "branch_scope",
  });
  if (scope !== "selected") return null;
  return (
    <AppMultiCombobox
      name="branch_uuids"
      label={t("tenant_members.fields.branches")}
      description={t("tenant_members.fields.branches_hint")}
      options={branchOptions}
      placeholder={t("tenant_members.fields.branches_placeholder")}
      emptyText={t("tenant_members.fields.branches_empty")}
    />
  );
}

/**
 * Drawer to change a member's roles (several; permissions are their union)
 * and branch access (every branch, or only the selected ones).
 */
export function MemberAccessDrawer({
  member,
  onOpenChange,
}: {
  member: TenantMember | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { branchScope } = useTenant();
  const update = useUpdateTenantMember();
  const schema = useMemo(() => createMemberAccessSchema(t), [t]);
  const rolesQuery = useAllTenantRoles(Boolean(member));
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
  const branchOptions = useMemo<MultiComboboxOption[]>(() => {
    const options = (branchesQuery.data?.items ?? []).map((branch) => ({
      value: branch.uuid,
      label: `${branch.code} · ${branch.name}`,
      hint: branch.city || undefined,
    }));
    // Keep granted branches selectable even when outside the first page.
    for (const branch of member?.branches ?? []) {
      if (!options.some((option) => option.value === branch.uuid)) {
        options.push({
          value: branch.uuid,
          label: `${branch.code} · ${branch.name}`,
          hint: undefined,
        });
      }
    }
    return options;
  }, [branchesQuery.data?.items, member?.branches]);

  const fullName = member
    ? `${member.user.name} ${member.user.surname}`.trim() || member.user.email
    : "";

  return (
    <EntityDrawer
      open={Boolean(member)}
      onOpenChange={onOpenChange}
      title={t("tenant_members.access.title")}
      description={
        member
          ? t("tenant_members.access.description", {
              name: fullName,
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
          <Button type="submit" form={FORM_ID} disabled={update.isPending}>
            {t("tenant_members.actions.save")}
          </Button>
        </div>
      }
    >
      {member ? (
        <AppForm<MemberAccessValues>
          key={member.user.uuid}
          id={FORM_ID}
          className="space-y-5 px-4"
          schema={schema}
          defaultValues={memberAccessDefaults(member)}
          onSubmit={async (values) => {
            await update.mutateAsync({
              userUuid: member.user.uuid,
              body: memberAccessRequest(values),
            });
            onOpenChange(false);
          }}
        >
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
          {branchScope === "selected" ? (
            <Alert>
              <AlertDescription>
                {t("tenant_members.access.restricted_hint")}
              </AlertDescription>
            </Alert>
          ) : null}
          <BranchPicker branchOptions={branchOptions} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

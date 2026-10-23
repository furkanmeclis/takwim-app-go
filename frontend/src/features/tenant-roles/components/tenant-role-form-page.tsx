"use client";

import { Shield, ShieldCheck } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo } from "react";
import type { UseFormReturn } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import {
  PermissionSelector,
  tenantPermissionGroupKey,
} from "@/components/permissions";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useCreateTenantRole,
  useTenantPermissionCatalog,
  useTenantRole,
  useUpdateTenantRole,
} from "@/features/tenant-roles/hooks/use-tenant-roles";
import {
  createTenantRoleSchema,
  roleKeyFromName,
  tenantRoleDefaults,
  tenantRoleRequest,
  type TenantRoleFormValues,
} from "@/features/tenant-roles/schemas/tenant-role-form";
import type { TenantRole } from "@/features/tenant-roles/types";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

type Mode = "create" | "edit" | "view";

function RoleFields({
  form,
  mode,
  catalog,
  catalogLoading,
  onCancel,
}: {
  form: UseFormReturn<TenantRoleFormValues>;
  mode: Mode;
  catalog: { slug: string; name: string }[];
  catalogLoading: boolean;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const readOnly = mode === "view";
  const name = form.watch("name");
  const selected = form.watch("permissions");

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "details",
          label: t("tenant_roles.form.section_details"),
          description: t("tenant_roles.form.section_details_desc"),
          icon: Shield,
        },
        {
          key: "permissions",
          label: t("tenant_roles.form.section_permissions"),
          description: t("tenant_roles.form.section_permissions_desc"),
          icon: ShieldCheck,
        },
      ]),
    [t],
  );

  // The key follows the name until the user edits it (create only).
  useEffect(() => {
    if (mode !== "create" || form.formState.dirtyFields.key) return;
    const next = roleKeyFromName(name);
    if (form.getValues("key") !== next) {
      form.setValue("key", next, { shouldDirty: false });
    }
  }, [form, mode, name]);

  return (
    <FormLayout navItems={sections.navItems}>
      <FormSection
        id={sections.id("details")}
        title={t("tenant_roles.form.section_details")}
        description={t("tenant_roles.form.section_details_desc")}
        columns={2}
      >
        <AppInput
          name="name"
          label={t("tenant_roles.fields.name")}
          disabled={readOnly}
        />
        <AppInput
          name="key"
          label={t("tenant_roles.fields.key")}
          description={t("tenant_roles.fields.key_hint")}
          disabled={mode !== "create"}
        />
        <AppTextarea
          name="description"
          label={t("tenant_roles.fields.description")}
          className="sm:col-span-2"
          disabled={readOnly}
        />
      </FormSection>

      <FormSection
        id={sections.id("permissions")}
        title={t("tenant_roles.form.section_permissions")}
        description={t("tenant_roles.form.section_permissions_desc")}
      >
        {catalogLoading ? (
          <Loading label={t("common.loading")} />
        ) : (
          <PermissionSelector
            permissions={catalog}
            value={selected}
            disabled={readOnly}
            groupBy={tenantPermissionGroupKey}
            onChange={(next) =>
              form.setValue("permissions", next, { shouldDirty: true })
            }
          />
        )}
      </FormSection>

      {readOnly ? null : (
        <FormActions>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" disabled={form.formState.isSubmitting}>
            {mode === "create"
              ? t("tenant_roles.actions.create")
              : t("tenant_roles.actions.save")}
          </Button>
        </FormActions>
      )}
    </FormLayout>
  );
}

function RoleForm({ mode, role }: { mode: Mode; role?: TenantRole }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const schema = useMemo(() => createTenantRoleSchema(t), [t]);
  const catalogQuery = useTenantPermissionCatalog();
  const create = useCreateTenantRole();
  const update = useUpdateTenantRole();

  const catalog = useMemo(
    () =>
      (catalogQuery.data?.items ?? []).map((permission) => {
        const key = `permissions.labels.${permission.slug}`;
        const label = t(key);
        return {
          slug: permission.slug,
          name: label === key ? permission.name : label,
        };
      }),
    [catalogQuery.data?.items, t],
  );
  const backToList = () => router.push(routes.tenant.settings.roles.root(slug));

  return (
    <AppForm<TenantRoleFormValues>
      key={role?.uuid ?? "new"}
      schema={schema}
      defaultValues={tenantRoleDefaults(role)}
      onSubmit={async (values) => {
        if (mode === "create") {
          await create.mutateAsync(tenantRoleRequest(values, "create"));
        } else if (role) {
          await update.mutateAsync({
            uuid: role.uuid,
            body: tenantRoleRequest(values, "edit"),
          });
        }
        backToList();
      }}
    >
      {(form) => (
        <RoleFields
          form={form}
          mode={mode}
          catalog={catalog}
          catalogLoading={catalogQuery.isLoading}
          onCancel={backToList}
        />
      )}
    </AppForm>
  );
}

/** Create page of an organization role. */
export function TenantRoleCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("tenant_roles.create_title")}
      description={t("tenant_roles.create_description")}
      permission={permissions.tenant.roles.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_roles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("tenant_roles.title"),
          href: routes.tenant.settings.roles.root(slug),
        },
        { label: t("tenant_roles.create_title") },
      ]}
    >
      <RoleForm mode="create" />
    </EntityPage>
  );
}

/**
 * Role page: organization roles are edited in place (with
 * `tenant.roles.write`); template roles and read-only callers see the matrix
 * disabled.
 */
export function TenantRoleDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useTenantRole(uuid);
  const role = query.data;
  const title = role
    ? tenantRoleLabel(t, role)
    : t("tenant_roles.detail_title");
  const mode: Mode =
    role && !role.is_template && can(permissions.tenant.roles.write)
      ? "edit"
      : "view";

  return (
    <EntityPage
      title={title}
      description={
        role?.is_template
          ? t("tenant_roles.template_description")
          : t("tenant_roles.detail_description")
      }
      permission={permissions.tenant.roles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_roles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("tenant_roles.title"),
          href: routes.tenant.settings.roles.root(slug),
        },
        { label: title },
      ]}
    >
      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError || !role ? (
        <ErrorState
          title={t("tenant_roles.not_found_title")}
          description={t("tenant_roles.not_found_description")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <div className="space-y-4">
          {role.is_template ? (
            <Alert>
              <AlertDescription>
                {t("tenant_roles.template_read_only")}
              </AlertDescription>
            </Alert>
          ) : null}
          <RoleForm mode={mode} role={role} />
        </div>
      )}
    </EntityPage>
  );
}

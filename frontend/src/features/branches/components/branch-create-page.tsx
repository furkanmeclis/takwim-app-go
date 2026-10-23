"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BranchForm } from "@/features/branches/components/branch-form";
import { useCreateBranch } from "@/features/branches/hooks/use-branches";
import {
  branchFormDefaults,
  branchFormToRequest,
} from "@/features/branches/schemas/branch-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function BranchCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateBranch();

  return (
    <EntityPage
      title={t("branches.create_title")}
      description={t("branches.create_description")}
      permission={permissions.tenant.branches.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("branches.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("branches.title"), href: routes.tenant.branches.root(slug) },
        { label: t("branches.create_title") },
      ]}
    >
      <EntityForm>
        <BranchForm
          mode="create"
          defaultValues={branchFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.branches.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              branchFormToRequest(values),
            );
            router.push(routes.tenant.branches.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

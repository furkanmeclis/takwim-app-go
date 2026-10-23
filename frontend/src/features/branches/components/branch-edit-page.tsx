"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BranchForm } from "@/features/branches/components/branch-form";
import {
  useBranch,
  useUpdateBranch,
} from "@/features/branches/hooks/use-branches";
import {
  branchFormDefaults,
  branchFormToRequest,
} from "@/features/branches/schemas/branch-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function BranchEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useBranch(uuid);
  const update = useUpdateBranch();
  const branch = query.data;
  const title = branch
    ? t("branches.edit_title_with_name", { name: branch.name })
    : t("branches.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("branches.edit_description")}
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
        {
          label: branch?.name ?? t("branches.detail_title"),
          href: routes.tenant.branches.detail(slug, uuid),
        },
        { label: t("branches.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("branches.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {branch ? (
        <EntityForm>
          <BranchForm
            key={branch.updated_at}
            mode="edit"
            defaultValues={branchFormDefaults(branch)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.branches.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: branchFormToRequest(values),
              });
              router.push(routes.tenant.branches.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

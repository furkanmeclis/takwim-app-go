"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceForm } from "@/features/resources/components/resource-form";
import {
  useResource,
  useUpdateResource,
} from "@/features/resources/hooks/use-resources";
import {
  resourceFormDefaults,
  resourceFormToRequest,
} from "@/features/resources/schemas/resource-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function ResourceEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useResource(uuid);
  const update = useUpdateResource();
  const resource = query.data;
  const title = resource
    ? t("resources.edit_title_with_name", { name: resource.name })
    : t("resources.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("resources.edit_description")}
      permission={permissions.tenant.resources.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("resources.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("resources.title"),
          href: routes.tenant.resources.root(slug),
        },
        {
          label: resource?.name ?? t("resources.detail_title"),
          href: routes.tenant.resources.detail(slug, uuid),
        },
        { label: t("resources.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("resources.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {resource ? (
        <EntityForm>
          <ResourceForm
            key={resource.updated_at}
            mode="edit"
            defaultValues={resourceFormDefaults(resource)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.resources.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: resourceFormToRequest(values),
              });
              router.push(routes.tenant.resources.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

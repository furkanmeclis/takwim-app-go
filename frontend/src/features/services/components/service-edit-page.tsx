"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ServiceForm } from "@/features/services/components/service-form";
import {
  useService,
  useUpdateService,
} from "@/features/services/hooks/use-services";
import {
  serviceFormDefaults,
  serviceFormToRequest,
} from "@/features/services/schemas/service-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function ServiceEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useService(uuid);
  const update = useUpdateService();
  const service = query.data;
  const title = service
    ? t("services.edit_title_with_name", { name: service.name })
    : t("services.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("services.edit_description")}
      permission={permissions.tenant.services.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("services.title"),
          href: routes.tenant.services.root(slug),
        },
        {
          label: service?.name ?? t("services.detail_title"),
          href: routes.tenant.services.detail(slug, uuid),
        },
        { label: t("services.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("services.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {service ? (
        <EntityForm>
          <ServiceForm
            key={service.updated_at}
            mode="edit"
            defaultValues={serviceFormDefaults(service)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.services.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              // Offers change on the detail page (PUT /offers).
              const { branch_uuids: _branches, ...body } =
                serviceFormToRequest(values);
              await update.mutateAsync({ uuid, body });
              router.push(routes.tenant.services.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BundleForm } from "@/features/service-bundles/components/bundle-form";
import {
  useServiceBundle,
  useUpdateServiceBundle,
} from "@/features/service-bundles/hooks/use-service-bundles";
import {
  bundleFormDefaults,
  bundleFormToRequest,
} from "@/features/service-bundles/schemas/bundle-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function BundleEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useServiceBundle(uuid);
  const update = useUpdateServiceBundle();
  const bundle = query.data;
  const title = bundle
    ? t("service_bundles.edit_title_with_name", { name: bundle.name })
    : t("service_bundles.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("service_bundles.edit_description")}
      permission={permissions.tenant.services.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("service_bundles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("service_bundles.title"),
          href: routes.tenant.serviceBundles.root(slug),
        },
        {
          label: bundle?.name ?? t("service_bundles.detail_title"),
          href: routes.tenant.serviceBundles.detail(slug, uuid),
        },
        { label: t("service_bundles.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("service_bundles.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {bundle ? (
        <EntityForm>
          <BundleForm
            key={bundle.updated_at}
            mode="edit"
            defaultValues={bundleFormDefaults(bundle)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.serviceBundles.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: bundleFormToRequest(values),
              });
              router.push(routes.tenant.serviceBundles.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

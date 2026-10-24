"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BundleForm } from "@/features/service-bundles/components/bundle-form";
import { useCreateServiceBundle } from "@/features/service-bundles/hooks/use-service-bundles";
import {
  bundleFormDefaults,
  bundleFormToRequest,
} from "@/features/service-bundles/schemas/bundle-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useLocale } from "@/providers/locale-provider";

export function BundleCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateServiceBundle();
  // A branch picked in the header offers the new bundle from the start.
  const [selectedBranch] = useSelectedBranch(slug);

  return (
    <EntityPage
      title={t("service_bundles.create_title")}
      description={t("service_bundles.create_description")}
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
        { label: t("service_bundles.create_title") },
      ]}
    >
      <EntityForm>
        <BundleForm
          mode="create"
          defaultValues={bundleFormDefaults(undefined, {
            branchUuids: selectedBranch ? [selectedBranch] : [],
          })}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.serviceBundles.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              bundleFormToRequest(values),
            );
            router.push(
              routes.tenant.serviceBundles.detail(slug, created.uuid),
            );
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceForm } from "@/features/resources/components/resource-form";
import { useCreateResource } from "@/features/resources/hooks/use-resources";
import { useResourceKinds } from "@/features/resources/lib/kinds";
import {
  resourceFormDefaults,
  resourceFormToRequest,
} from "@/features/resources/schemas/resource-form";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function ResourceCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateResource();
  const kinds = useResourceKinds();
  // A branch picked in the header is the natural home of a new resource.
  const [selectedBranch] = useSelectedBranch(slug);
  const defaults = resourceFormDefaults(undefined, {
    branchUuid: selectedBranch ?? undefined,
    kind: kinds.primary,
  });

  return (
    <EntityPage
      title={t("resources.create_title")}
      description={t("resources.create_description")}
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
        { label: t("resources.create_title") },
      ]}
    >
      <EntityForm>
        <ResourceForm
          mode="create"
          defaultValues={defaults}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.resources.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              resourceFormToRequest(values),
            );
            router.push(routes.tenant.resources.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

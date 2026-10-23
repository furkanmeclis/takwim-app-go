"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { ServiceForm } from "@/features/services/components/service-form";
import { useCreateService } from "@/features/services/hooks/use-services";
import {
  serviceFormDefaults,
  serviceFormToRequest,
} from "@/features/services/schemas/service-form";
import type { StaffMode } from "@/features/services/types";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function ServiceCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { sector } = useSector();
  const router = useRouter();
  const create = useCreateService();
  // A branch picked in the header offers the new service from the start.
  const [selectedBranch] = useSelectedBranch(slug);
  const defaults = serviceFormDefaults(undefined, {
    staffMode: sector?.capacity.staff as StaffMode | undefined,
    branchUuids: selectedBranch ? [selectedBranch] : [],
  });

  return (
    <EntityPage
      title={t("services.create_title")}
      description={t("services.create_description")}
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
        { label: t("services.create_title") },
      ]}
    >
      <EntityForm>
        <ServiceForm
          key={sector?.code ?? "default"}
          mode="create"
          defaultValues={defaults}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.services.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              serviceFormToRequest(values),
            );
            router.push(routes.tenant.services.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

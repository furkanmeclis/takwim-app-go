"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { StaffForm } from "@/features/staff/components/staff-form";
import { useCreateStaff } from "@/features/staff/hooks/use-staff";
import {
  staffFormDefaults,
  staffFormToRequest,
} from "@/features/staff/schemas/staff-form";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function StaffCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateStaff();
  // A branch picked in the header is the natural first assignment.
  const [selectedBranch] = useSelectedBranch(slug);
  const defaults = {
    ...staffFormDefaults(),
    branch_uuids: selectedBranch ? [selectedBranch] : [],
  };

  return (
    <EntityPage
      title={t("staff.create_title")}
      description={t("staff.create_description")}
      permission={permissions.tenant.staff.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("staff.title"), href: routes.tenant.staff.root(slug) },
        { label: t("staff.create_title") },
      ]}
    >
      <EntityForm>
        <StaffForm
          mode="create"
          defaultValues={defaults}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.staff.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              staffFormToRequest(values),
            );
            router.push(routes.tenant.staff.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

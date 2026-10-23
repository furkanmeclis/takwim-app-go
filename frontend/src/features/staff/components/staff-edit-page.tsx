"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { StaffForm } from "@/features/staff/components/staff-form";
import {
  useStaffMember,
  useUpdateStaff,
} from "@/features/staff/hooks/use-staff";
import {
  staffFormDefaults,
  staffFormToRequest,
} from "@/features/staff/schemas/staff-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function StaffEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useStaffMember(uuid);
  const update = useUpdateStaff();
  const staff = query.data;
  const title = staff
    ? t("staff.edit_title_with_name", { name: staff.name })
    : t("staff.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("staff.edit_description")}
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
        {
          label: staff?.name ?? t("staff.detail_title"),
          href: routes.tenant.staff.detail(slug, uuid),
        },
        { label: t("staff.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("staff.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {staff ? (
        <EntityForm>
          <StaffForm
            key={staff.updated_at}
            mode="edit"
            defaultValues={staffFormDefaults(staff)}
            currentAccount={staff.account}
            isSubmitting={update.isPending}
            onCancel={() => router.push(routes.tenant.staff.detail(slug, uuid))}
            onSubmit={async (values) => {
              const body = staffFormToRequest(values);
              // Keep the link untouched when the choice did not change.
              if ((staff.account?.uuid ?? null) === body.account_uuid) {
                delete body.account_uuid;
              }
              await update.mutateAsync({ uuid, body });
              router.push(routes.tenant.staff.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

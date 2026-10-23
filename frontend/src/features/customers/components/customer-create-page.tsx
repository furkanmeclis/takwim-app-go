"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CustomerForm } from "@/features/customers/components/customer-form";
import { useCreateCustomer } from "@/features/customers/hooks/use-customers";
import {
  customerFormDefaults,
  customerFormToRequest,
} from "@/features/customers/schemas/customer-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function CustomerCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateCustomer();

  return (
    <EntityPage
      title={t("customers.create_title")}
      description={t("customers.create_description")}
      permission={permissions.tenant.customers.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.title"),
          href: routes.tenant.customers.root(slug),
        },
        { label: t("customers.create_title") },
      ]}
    >
      <EntityForm>
        <CustomerForm
          mode="create"
          defaultValues={customerFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.customers.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              customerFormToRequest(values),
            );
            router.push(routes.tenant.customers.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

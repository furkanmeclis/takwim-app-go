"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CustomerForm } from "@/features/customers/components/customer-form";
import {
  useCustomer,
  useUpdateCustomer,
} from "@/features/customers/hooks/use-customers";
import {
  customerFormDefaults,
  customerFormToRequest,
} from "@/features/customers/schemas/customer-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function CustomerEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useCustomer(uuid);
  const update = useUpdateCustomer();
  const customer = query.data;
  const title = customer
    ? t("customers.edit_title_with_name", { name: customer.full_name })
    : t("customers.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("customers.edit_description")}
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
        {
          label: customer?.full_name ?? t("customers.detail_title"),
          href: routes.tenant.customers.detail(slug, uuid),
        },
        { label: t("customers.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("customers.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {customer ? (
        <EntityForm>
          <CustomerForm
            key={customer.updated_at}
            mode="edit"
            defaultValues={customerFormDefaults(customer)}
            verified={Boolean(customer.verification)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.customers.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: customerFormToRequest(values),
              });
              router.push(routes.tenant.customers.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

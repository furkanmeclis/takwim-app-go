"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ProductForm } from "@/features/products/components/product-form";
import { useCreateProduct } from "@/features/products/hooks/use-products";
import { localizeProductError } from "@/features/products/lib/format";
import {
  productFormDefaults,
  productFormToRequest,
} from "@/features/products/schemas/product-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useLocale } from "@/providers/locale-provider";

export function ProductCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateProduct();
  // A branch picked in the header sells the new product from the start.
  const [selectedBranch] = useSelectedBranch(slug);
  const defaults = productFormDefaults(undefined, {
    branchUuids: selectedBranch ? [selectedBranch] : [],
  });

  return (
    <EntityPage
      title={t("products.create_title")}
      description={t("products.create_description")}
      permission={permissions.tenant.products.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("products.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("products.title"),
          href: routes.tenant.products.root(slug),
        },
        { label: t("products.create_title") },
      ]}
    >
      <EntityForm>
        <ProductForm
          key={selectedBranch ?? "all"}
          mode="create"
          defaultValues={defaults}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.products.root(slug))}
          onSubmit={async (values) => {
            try {
              const created = await create.mutateAsync(
                productFormToRequest(values),
              );
              router.push(routes.tenant.products.detail(slug, created.uuid));
            } catch (error) {
              localizeProductError(error, t);
              throw error;
            }
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

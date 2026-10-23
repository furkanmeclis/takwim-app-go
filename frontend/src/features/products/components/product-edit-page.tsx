"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ProductForm } from "@/features/products/components/product-form";
import {
  useProduct,
  useUpdateProduct,
} from "@/features/products/hooks/use-products";
import { localizeProductError } from "@/features/products/lib/format";
import {
  productFormDefaults,
  productFormToRequest,
} from "@/features/products/schemas/product-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export function ProductEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useProduct(uuid);
  const update = useUpdateProduct();
  const product = query.data;
  const title = product
    ? t("products.edit_title_with_name", { name: product.name })
    : t("products.edit_title");

  return (
    <EntityPage
      title={title}
      description={t("products.edit_description")}
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
        {
          label: product?.name ?? t("products.detail_title"),
          href: routes.tenant.products.detail(slug, uuid),
        },
        { label: t("products.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("products.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {product ? (
        <EntityForm>
          <ProductForm
            key={product.updated_at}
            mode="edit"
            defaultValues={productFormDefaults(product)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.products.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              // Offers change on the detail page (PUT /offers).
              const { branch_uuids: _branches, ...body } =
                productFormToRequest(values);
              try {
                await update.mutateAsync({ uuid, body });
              } catch (error) {
                localizeProductError(error, t);
                throw error;
              }
              router.push(routes.tenant.products.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

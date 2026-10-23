"use client";

import Link from "next/link";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { MAX_LOW_STOCK } from "@/features/products/constants";
import { useUpdateProduct } from "@/features/products/hooks/use-products";
import {
  createQuickEditSchema,
  normalizePrice,
  type QuickEditValues,
} from "@/features/products/schemas/product-form";
import type { Product } from "@/features/products/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "product-quick-edit";

/** Drawer for the fields people change most, without leaving the list. */
export function ProductQuickEditDrawer({
  product,
  onOpenChange,
}: {
  product: Product | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateProduct();
  const schema = useMemo(() => createQuickEditSchema(t), [t]);

  return (
    <EntityDrawer
      open={Boolean(product)}
      onOpenChange={onOpenChange}
      title={t("products.quick_edit.title")}
      description={product?.name}
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {product ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.products.edit(slug, product.uuid)}>
                {t("products.quick_edit.full_form")}
              </Link>
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={update.isPending}>
              {t("products.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {product ? (
        <AppForm<QuickEditValues>
          key={product.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: product.name,
            price: product.price,
            low_stock_threshold: String(product.low_stock_threshold),
            show_online: product.show_online,
            is_active: product.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: product.uuid,
              body: {
                name: values.name.trim(),
                price: normalizePrice(values.price) ?? "0.00",
                low_stock_threshold: Number(values.low_stock_threshold),
                show_online: values.show_online,
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("products.fields.name")} />
          <div className="grid grid-cols-2 gap-4">
            <AppInput
              name="price"
              inputMode="decimal"
              label={t("products.fields.price_short")}
            />
            <AppInput
              name="low_stock_threshold"
              type="number"
              min={0}
              max={MAX_LOW_STOCK}
              label={t("products.fields.low_stock_threshold")}
            />
          </div>
          <AppSwitch
            name="show_online"
            label={t("products.fields.show_online")}
          />
          <AppSwitch name="is_active" label={t("products.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

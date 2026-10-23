"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { MAX_SORT_ORDER } from "@/features/products/constants";
import {
  createVariantSchema,
  variantFormDefaults,
  type VariantFormValues,
} from "@/features/products/schemas/product-form";
import type { Product, ProductVariant } from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "product-variant-form";

/**
 * Adds or edits a variant (size, color, volume …). Empty price / cost
 * means the product's own. Each variant keeps its own stock.
 */
export function ProductVariantDialog({
  open,
  product,
  variant,
  isPending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  product: Product;
  /** Variant to edit; `null` adds one. */
  variant: ProductVariant | null;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: VariantFormValues) => Promise<void>;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createVariantSchema(t), [t]);
  const next = (product.variants?.length ?? 0) * 10;

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        variant
          ? t("products.variants.edit_title", { name: variant.name })
          : t("products.variants.add_title")
      }
      description={
        variant || (product.variants?.length ?? 0) > 0
          ? t("products.variants.dialog_description")
          : t("products.variants.first_variant_hint")
      }
      className="sm:max-w-xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={isPending}>
            {variant ? t("products.actions.save") : t("products.variants.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<VariantFormValues>
          key={variant?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={variantFormDefaults(variant, next)}
          onSubmit={async (values) => {
            await onSubmit(values);
            onOpenChange(false);
          }}
        >
          <AppInput
            name="name"
            label={t("products.variants.fields.name")}
            description={t("products.variants.fields.name_hint")}
            className="sm:col-span-2"
          />
          <AppInput name="sku" label={t("products.fields.sku")} />
          <AppInput name="barcode" label={t("products.fields.barcode")} />
          <AppInput
            name="price"
            inputMode="decimal"
            label={t("products.variants.fields.price")}
            placeholder={product.price}
            description={t("products.variants.fields.price_hint")}
          />
          <AppInput
            name="cost"
            inputMode="decimal"
            label={t("products.variants.fields.cost")}
            placeholder={product.cost}
            description={t("products.variants.fields.price_hint")}
          />
          <AppInput
            name="sort_order"
            type="number"
            min={0}
            max={MAX_SORT_ORDER}
            label={t("products.variants.fields.sort_order")}
          />
          <AppSwitch
            name="is_active"
            label={t("products.variants.fields.is_active")}
            description={t("products.variants.fields.is_active_hint")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

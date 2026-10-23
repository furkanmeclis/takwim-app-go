"use client";

import { Boxes, Package, Store, Wallet } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  MAX_LOW_STOCK,
  PRODUCT_UNITS,
  UNIT_LABEL_KEYS,
} from "@/features/products/constants";
import {
  createProductFormSchema,
  type ProductFormValues,
} from "@/features/products/schemas/product-form";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type ProductFormProps = {
  mode: "create" | "edit";
  defaultValues: ProductFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: ProductFormValues) => Promise<void>;
  onCancel: () => void;
};

/**
 * Create / edit form: identity (name, stock code, barcode, unit), price
 * and cost, stock alert level. On create the branches selling the product
 * are picked too; later they change on the detail page (branch offers).
 * Stock itself only changes through movements.
 */
export function ProductForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: ProductFormProps) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const schema = useMemo(() => createProductFormSchema(t), [t]);
  const branches = useAccessibleBranches();
  const currency = organization?.currency ?? "TRY";

  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      })),
    [branches.data?.items],
  );
  const unitOptions = PRODUCT_UNITS.map((unit) => ({
    value: unit,
    label: t(UNIT_LABEL_KEYS[unit]),
  }));

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("products.form.section_identity"),
          description: t("products.form.section_identity_desc"),
          icon: Package,
        },
        {
          key: "price",
          label: t("products.form.section_price"),
          description: t("products.form.section_price_desc"),
          icon: Wallet,
        },
        {
          key: "stock",
          label: t("products.form.section_stock"),
          description: t("products.form.section_stock_desc"),
          icon: Boxes,
        },
        ...(mode === "create"
          ? [
              {
                key: "branches",
                label: t("products.form.section_branches"),
                description: t("products.form.section_branches_desc"),
                icon: Store,
              },
            ]
          : []),
      ]),
    [mode, t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("identity")}
            title={t("products.form.section_identity")}
            description={t("products.form.section_identity_desc")}
            columns={2}
          >
            <AppInput
              name="name"
              label={t("products.fields.name")}
              description={t("products.fields.name_hint")}
            />
            <AppSelect
              name="unit"
              label={t("products.fields.unit")}
              description={t("products.fields.unit_hint")}
              options={unitOptions}
            />
            <AppInput
              name="sku"
              label={t("products.fields.sku")}
              description={t("products.fields.sku_hint")}
            />
            <AppInput
              name="barcode"
              label={t("products.fields.barcode")}
              description={t("products.fields.barcode_hint")}
            />
            <AppSwitch
              name="show_online"
              label={t("products.fields.show_online")}
              description={t("products.fields.show_online_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("products.fields.is_active")}
              description={t("products.fields.is_active_hint")}
            />
            <AppTextarea
              name="description"
              label={t("products.fields.description")}
              description={t("products.fields.description_hint")}
              className="sm:col-span-2"
            />
          </FormSection>

          <FormSection
            id={sections.id("price")}
            title={t("products.form.section_price")}
            description={t("products.form.section_price_desc")}
            columns={2}
          >
            <AppInput
              name="price"
              inputMode="decimal"
              label={t("products.fields.price", { currency })}
              description={t("products.fields.price_hint")}
            />
            <AppInput
              name="cost"
              inputMode="decimal"
              label={t("products.fields.cost", { currency })}
              description={t("products.fields.cost_hint")}
            />
          </FormSection>

          <FormSection
            id={sections.id("stock")}
            title={t("products.form.section_stock")}
            description={t("products.form.section_stock_desc")}
            columns={2}
          >
            <AppInput
              name="low_stock_threshold"
              type="number"
              min={0}
              max={MAX_LOW_STOCK}
              label={t("products.fields.low_stock_threshold")}
              description={t("products.fields.low_stock_threshold_hint")}
            />
          </FormSection>

          {mode === "create" ? (
            <FormSection
              id={sections.id("branches")}
              title={t("products.form.section_branches")}
              description={t("products.form.section_branches_desc")}
            >
              <AppMultiCombobox
                name="branch_uuids"
                label={t("products.fields.branches")}
                description={t("products.fields.branches_hint")}
                options={branchOptions}
                placeholder={t("products.fields.branches_placeholder")}
                emptyText={t("products.fields.branches_empty")}
              />
            </FormSection>
          ) : null}

          <FormActions>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || form.formState.isSubmitting}
            >
              {mode === "create"
                ? t("products.actions.create")
                : t("products.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}

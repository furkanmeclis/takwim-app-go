"use client";

import {
  ArrowDown,
  ArrowUp,
  Layers,
  ListOrdered,
  Plus,
  Store,
  Trash2,
  Wallet,
} from "lucide-react";
import { useMemo } from "react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";

import {
  AppCombobox,
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
  type ComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { MAX_BUNDLE_ITEMS } from "@/features/service-bundles/constants";
import {
  createBundleFormSchema,
  type BundleFormValues,
} from "@/features/service-bundles/schemas/bundle-form";
import { useServicesList } from "@/features/services/hooks/use-services";
import { priceLabel } from "@/features/services/lib/format";
import type { Service } from "@/features/services/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type BundleFormProps = {
  mode: "create" | "edit";
  defaultValues: BundleFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: BundleFormValues) => Promise<void>;
  onCancel: () => void;
};

/**
 * Create / edit form of a combined service bundle: identity, the services
 * in the order they are served, the bundle price (with the list total and
 * the saving) and the branches that offer it.
 */
export function BundleForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: BundleFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createBundleFormSchema(t), [t]);
  const branches = useAccessibleBranches();
  // Group classes book seats of a session and cannot be bundled.
  const services = useServicesList({ limit: 100, offset: 0, sort: "name" });
  const catalog = useMemo(
    () =>
      (services.data?.items ?? []).filter(
        (service) => service.group_capacity == null,
      ),
    [services.data?.items],
  );
  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      })),
    [branches.data?.items],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("service_bundles.form.section_identity"),
          icon: Layers,
        },
        {
          key: "items",
          label: t("service_bundles.form.section_items"),
          icon: ListOrdered,
        },
        {
          key: "price",
          label: t("service_bundles.form.section_price"),
          icon: Wallet,
        },
        {
          key: "branches",
          label: t("service_bundles.form.section_branches"),
          icon: Store,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("identity")}
            title={t("service_bundles.form.section_identity")}
            description={t("service_bundles.form.section_identity_desc")}
            columns={2}
          >
            <AppInput
              name="name"
              label={t("service_bundles.fields.name")}
              description={t("service_bundles.fields.name_hint")}
            />
            <AppInput
              name="sort_order"
              type="number"
              min={0}
              label={t("service_bundles.fields.sort_order")}
              description={t("service_bundles.fields.sort_order_hint")}
            />
            <AppSwitch
              name="show_online"
              label={t("service_bundles.fields.show_online")}
              description={t("service_bundles.fields.show_online_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("service_bundles.fields.is_active")}
              description={t("service_bundles.fields.is_active_hint")}
            />
            <AppTextarea
              name="description"
              label={t("service_bundles.fields.description")}
              description={t("service_bundles.fields.description_hint")}
              className="sm:col-span-2"
            />
          </FormSection>

          <FormSection
            id={sections.id("items")}
            title={t("service_bundles.form.section_items")}
            description={t("service_bundles.form.section_items_desc")}
          >
            <BundleItemsField catalog={catalog} />
          </FormSection>

          <FormSection
            id={sections.id("price")}
            title={t("service_bundles.form.section_price")}
            description={t("service_bundles.form.section_price_desc")}
          >
            <BundlePriceField catalog={catalog} />
          </FormSection>

          <FormSection
            id={sections.id("branches")}
            title={t("service_bundles.form.section_branches")}
            description={t("service_bundles.form.section_branches_desc")}
          >
            <AppMultiCombobox
              name="branch_uuids"
              label={t("service_bundles.fields.branches")}
              description={t("service_bundles.fields.branches_hint")}
              options={branchOptions}
              placeholder={t("service_bundles.fields.branches_placeholder")}
              emptyText={t("service_bundles.fields.branches_empty")}
            />
          </FormSection>

          <FormActions>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || form.formState.isSubmitting}
            >
              {mode === "create"
                ? t("service_bundles.actions.create")
                : t("service_bundles.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}

/** The services of the bundle, in order: pick, reorder, remove. */
function BundleItemsField({ catalog }: { catalog: Service[] }) {
  const { t, locale } = useLocale();
  const form = useFormContext<BundleFormValues>();
  const items = useFieldArray({ control: form.control, name: "items" });
  const options = useMemo<ComboboxOption[]>(
    () =>
      catalog.map((service) => ({
        value: service.uuid,
        label: service.name,
        description: `${t("services.minutes", {
          minutes: String(service.duration_minutes),
        })} · ${priceLabel(service.price, service.currency, locale)}`,
      })),
    [catalog, locale, t],
  );
  const error = form.formState.errors.items;

  return (
    <div className="space-y-3">
      {items.fields.map((field, index) => (
        <div
          key={field.id}
          className="grid items-end gap-3 rounded-md border p-3 sm:grid-cols-[auto_1fr_auto]"
        >
          <span className="text-muted-foreground pb-2 text-sm font-medium">
            {t("service_bundles.items.position", { position: index + 1 })}
          </span>
          <AppCombobox
            name={`items.${index}.service_uuid`}
            label={t("service_bundles.fields.service")}
            placeholder={t("service_bundles.fields.service_placeholder")}
            options={options}
          />
          <div className="flex gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("service_bundles.items.move_up")}
              disabled={index === 0}
              onClick={() => items.move(index, index - 1)}
            >
              <ArrowUp className="size-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("service_bundles.items.move_down")}
              disabled={index === items.fields.length - 1}
              onClick={() => items.move(index, index + 1)}
            >
              <ArrowDown className="size-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("service_bundles.items.remove")}
              disabled={items.fields.length <= 1}
              onClick={() => items.remove(index)}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={items.fields.length >= MAX_BUNDLE_ITEMS}
        onClick={() => items.append({ service_uuid: "" })}
      >
        <Plus className="size-4" />
        {t("service_bundles.items.add")}
      </Button>
      {error?.message || error?.root?.message ? (
        <p className="text-destructive text-sm">
          {error.message ?? error.root?.message}
        </p>
      ) : null}
    </div>
  );
}

/** Bundle price with the catalog total of the picked services. */
function BundlePriceField({ catalog }: { catalog: Service[] }) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const currency = organization?.currency ?? "TRY";
  const [items, price] = useWatch<BundleFormValues, ["items", "price"]>({
    name: ["items", "price"],
  });
  const byUuid = useMemo(
    () => new Map(catalog.map((service) => [service.uuid, service])),
    [catalog],
  );
  const list = (items ?? []).reduce(
    (sum, item) => sum + Number(byUuid.get(item.service_uuid)?.price ?? 0),
    0,
  );
  const minutes = (items ?? []).reduce(
    (sum, item) => sum + (byUuid.get(item.service_uuid)?.duration_minutes ?? 0),
    0,
  );
  const bundlePrice = Number((price ?? "").replace(",", "."));
  const saving = Number.isFinite(bundlePrice) ? list - bundlePrice : 0;

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <AppInput
        name="price"
        inputMode="decimal"
        label={t("service_bundles.fields.price", { currency })}
        description={t("service_bundles.fields.price_hint")}
      />
      <dl className="bg-muted/40 grid grid-cols-2 gap-x-4 gap-y-1 rounded-md border p-3 text-sm">
        <dt className="text-muted-foreground">
          {t("service_bundles.summary.list_price")}
        </dt>
        <dd className="text-right font-medium">
          {priceLabel(list.toFixed(2), currency, locale)}
        </dd>
        <dt className="text-muted-foreground">
          {t("service_bundles.summary.discount")}
        </dt>
        <dd className="text-right font-medium">
          {priceLabel(Math.max(0, saving).toFixed(2), currency, locale)}
        </dd>
        <dt className="text-muted-foreground">
          {t("service_bundles.summary.duration")}
        </dt>
        <dd className="text-right font-medium">
          {t("services.minutes", { minutes: String(minutes) })}
        </dd>
      </dl>
    </div>
  );
}

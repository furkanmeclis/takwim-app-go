"use client";

import { useQuery } from "@tanstack/react-query";
import { ScanBarcode } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  AppToggleGroup,
  type ComboboxOption,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { permissions } from "@/config/permissions";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import {
  MAX_QUANTITY,
  MOVEMENT_REQUEST_KIND_LABEL_KEYS,
  MOVEMENT_REQUEST_KINDS,
} from "@/features/products/constants";
import {
  useProduct,
  useRecordStockMovement,
  useStockPaymentAccounts,
} from "@/features/products/hooks/use-products";
import {
  accountOptions,
  localizeProductError,
} from "@/features/products/lib/format";
import {
  COLLECT_METHODS,
  createMovementSchema,
  movementFormDefaults,
  movementFormToRequest,
  NO_STAFF,
  type MovementFormValues,
} from "@/features/products/schemas/movement-form";
import { productsService } from "@/features/products/services/products.service";
import type {
  MovementRequestKind,
  Product,
  ProductVariant,
} from "@/features/products/types";
import { staffKeys } from "@/features/staff/hooks/query-keys";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { staffService } from "@/features/staff/services/staff.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useSelectedBranch } from "@/features/tenant/hooks/use-selected-branch";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const FORM_ID = "stock-movement-form";

type MovementDialogDefaults = {
  kind?: MovementRequestKind;
  product?: Product | null;
  variantUuid?: string;
  branchUuid?: string;
};

function productOption(product: Product): ComboboxOption {
  return {
    value: product.uuid,
    label: product.name,
    description: product.sku,
  };
}

function variantLabel(variant: ProductVariant) {
  return `${variant.name} (${variant.sku})`;
}

/**
 * Records one stock movement: goods in, a sale, a return, a count or a
 * transfer to another branch. A barcode or stock code finds the product
 * (and variant). The API keeps stock from going below zero and answers
 * with field errors this form shows in place.
 */
export function StockMovementDialog({
  open,
  defaults,
  onOpenChange,
}: {
  open: boolean;
  defaults?: MovementDialogDefaults;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const [selectedBranch] = useSelectedBranch(slug);
  const record = useRecordStockMovement();
  const schema = useMemo(() => createMovementSchema(t), [t]);

  const canStock = can(permissions.tenant.products.stock);
  const canSell = can(permissions.tenant.products.sell);
  // Money with the sale (TEC-992): the collection permission and the cash
  // module, like a collection on a visit.
  const canCollect =
    canSell && can(permissions.tenant.payment.collect) && hasModule("cash");
  // A sale takes `tenant.products.sell`; every other kind `.stock`.
  const kinds = MOVEMENT_REQUEST_KINDS.filter((kind) =>
    kind === "sale" ? canSell : canStock,
  );
  const initialKind =
    defaults?.kind && kinds.includes(defaults.kind)
      ? defaults.kind
      : (kinds[0] ?? "receipt");

  const formDefaults = movementFormDefaults({
    kind: initialKind,
    productUuid: defaults?.product?.uuid,
    variantUuid: defaults?.variantUuid,
    branchUuid: defaults?.branchUuid ?? selectedBranch ?? undefined,
    collect: canCollect,
  });

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("products.movements.dialog_title")}
      description={
        defaults?.product
          ? defaults.product.name
          : t("products.movements.dialog_description")
      }
      className="sm:max-w-2xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={record.isPending}>
            {t("products.movements.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<MovementFormValues>
          key={`${defaults?.product?.uuid ?? "any"}-${defaults?.kind ?? ""}`}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={formDefaults}
          onSubmit={async (values) => {
            try {
              await record.mutateAsync(movementFormToRequest(values));
            } catch (error) {
              localizeProductError(error, t);
              throw error;
            }
            onOpenChange(false);
          }}
        >
          <MovementFields
            kinds={kinds}
            fixedProduct={defaults?.product}
            canCollect={canCollect}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

function MovementFields({
  kinds,
  fixedProduct,
  canCollect,
}: {
  kinds: MovementRequestKind[];
  fixedProduct?: Product | null;
  canCollect: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const form = useFormContext<MovementFormValues>();
  const [kind, productUuid, branchUuid, variantUuid] = useWatch({
    control: form.control,
    name: ["kind", "product_uuid", "branch_uuid", "variant_uuid"],
  });
  const [found, setFound] = useState<ComboboxOption[]>(
    fixedProduct ? [productOption(fixedProduct)] : [],
  );
  const [code, setCode] = useState("");
  const [looking, setLooking] = useState(false);

  // The detail carries variants, offers and per-branch levels.
  const productQuery = useProduct(productUuid || null);
  const product = productQuery.data ?? fixedProduct ?? null;
  const branches = useAccessibleBranches();
  const canSeeStaff = can(permissions.tenant.staff.read);
  const sells = kind === "sale" || kind === "return";
  const staffParams = {
    limit: 100,
    offset: 0,
    branch: branchUuid || undefined,
    is_active: "true" as const,
  };
  const staff = useQuery({
    queryKey: staffKeys.list(slug, staffParams),
    queryFn: () => staffService.list(slug, staffParams),
    enabled: canSeeStaff && sells,
    staleTime: 60_000,
  });

  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      })),
    [branches.data?.items],
  );
  const kindOptions = kinds.map((value) => ({
    value,
    label: t(MOVEMENT_REQUEST_KIND_LABEL_KEYS[value]),
  }));
  const variants = product?.variants ?? [];
  const variantOptions = variants.map((variant) => ({
    value: variant.uuid,
    label: variant.is_active
      ? variantLabel(variant)
      : `${variantLabel(variant)} · ${t("products.status.inactive")}`,
  }));
  const staffOptions = [
    { value: NO_STAFF, label: t("products.movements.fields.no_staff") },
    ...(canSeeStaff && sells ? (staff.data?.items ?? []) : []).map(
      (member) => ({ value: member.uuid, label: member.name }),
    ),
  ];

  // A product with variants needs one; a product without takes none.
  useEffect(() => {
    if (!product) return;
    const hasVariants = (product.variants?.length ?? 0) > 0;
    if (!hasVariants && variantUuid) form.setValue("variant_uuid", "");
    if (hasVariants && variantUuid) {
      const known = product.variants?.some((v) => v.uuid === variantUuid);
      if (!known) form.setValue("variant_uuid", "");
    }
  }, [form, product, variantUuid]);

  const level = product?.levels?.find(
    (row) =>
      row.branch_uuid === branchUuid &&
      (row.variant_uuid ?? "") === (variantUuid ?? ""),
  );
  const variant = variants.find((v) => v.uuid === variantUuid);
  const offer = product?.offers.find((o) => o.branch_uuid === branchUuid);
  const defaultPrice =
    variant?.price ?? offer?.price ?? product?.price ?? undefined;

  const loadProducts = useCallback(
    async (q: string) => {
      const page = await productsService.list(slug, {
        limit: 20,
        offset: 0,
        sort: "name",
        q: q || undefined,
      });
      return page.items.map(productOption);
    },
    [slug],
  );

  const lookup = async () => {
    const value = code.trim();
    if (!value) return;
    setLooking(true);
    try {
      const result = await productsService.lookup(slug, value);
      const match = result.items[0];
      if (!match) {
        appToast.error(t("products.movements.lookup_none", { code: value }));
        return;
      }
      setFound((prev) => [
        productOption(match.product),
        ...prev.filter((option) => option.value !== match.product.uuid),
      ]);
      form.setValue("product_uuid", match.product.uuid, {
        shouldValidate: true,
      });
      form.setValue("variant_uuid", match.variant?.uuid ?? "");
      setCode("");
    } finally {
      setLooking(false);
    }
  };

  return (
    <>
      <AppSelect
        name="kind"
        label={t("products.movements.fields.kind")}
        options={kindOptions}
      />
      <AppSelect
        name="branch_uuid"
        label={
          kind === "transfer"
            ? t("products.movements.fields.from_branch")
            : t("products.movements.fields.branch")
        }
        placeholder={t("products.movements.fields.branch_placeholder")}
        options={branchOptions}
      />
      {kind === "transfer" ? (
        <AppSelect
          name="to_branch_uuid"
          label={t("products.movements.fields.to_branch")}
          placeholder={t("products.movements.fields.branch_placeholder")}
          options={branchOptions.filter((o) => o.value !== branchUuid)}
          className="sm:col-span-2"
        />
      ) : null}

      {fixedProduct ? null : (
        <div className="space-y-2 sm:col-span-2">
          <div className="flex gap-2">
            <Input
              value={code}
              onChange={(event) => setCode(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  void lookup();
                }
              }}
              placeholder={t("products.movements.lookup_placeholder")}
              aria-label={t("products.movements.lookup_placeholder")}
            />
            <Button
              type="button"
              variant="outline"
              disabled={looking || !code.trim()}
              onClick={() => void lookup()}
            >
              <ScanBarcode className="size-4" />
              {t("products.movements.lookup")}
            </Button>
          </div>
          <AppCombobox
            name="product_uuid"
            label={t("products.movements.fields.product")}
            placeholder={t("products.movements.fields.product_placeholder")}
            searchPlaceholder={t("products.movements.fields.product_search")}
            emptyText={t("products.movements.fields.product_empty")}
            loadOptions={loadProducts}
            options={found}
          />
        </div>
      )}

      {variants.length > 0 ? (
        <AppSelect
          name="variant_uuid"
          label={t("products.movements.fields.variant")}
          placeholder={t("products.movements.fields.variant_placeholder")}
          options={variantOptions}
          className="sm:col-span-2"
        />
      ) : null}

      {kind === "adjustment" ? (
        <AppInput
          name="counted_quantity"
          type="number"
          min={0}
          max={MAX_QUANTITY}
          label={t("products.movements.fields.counted_quantity")}
          description={t("products.movements.fields.counted_hint", {
            on_hand: (level?.on_hand ?? 0).toLocaleString(locale),
          })}
        />
      ) : (
        <AppInput
          name="quantity"
          type="number"
          min={1}
          max={MAX_QUANTITY}
          label={t("products.movements.fields.quantity")}
          description={
            product
              ? t("products.movements.fields.available_hint", {
                  available: (level?.available ?? 0).toLocaleString(locale),
                })
              : undefined
          }
        />
      )}

      {sells ? (
        <AppInput
          name="unit_price"
          inputMode="decimal"
          label={t("products.movements.fields.unit_price")}
          placeholder={defaultPrice}
          description={t("products.movements.fields.unit_price_hint")}
        />
      ) : null}
      {kind === "receipt" ? (
        <AppInput
          name="unit_cost"
          inputMode="decimal"
          label={t("products.movements.fields.unit_cost")}
          placeholder={variant?.cost ?? product?.cost ?? undefined}
          description={t("products.movements.fields.unit_cost_hint")}
        />
      ) : null}
      {sells ? (
        <AppSelect
          name="staff_uuid"
          label={t("products.movements.fields.staff")}
          description={t("products.movements.fields.staff_hint")}
          options={staffOptions}
        />
      ) : null}

      <AppInput
        name="reason"
        label={t("products.movements.fields.reason")}
        placeholder={t("products.movements.fields.reason_placeholder")}
      />
      <AppDatePicker
        name="occurred_at"
        withTime
        label={t("products.movements.fields.occurred_at")}
        description={t("products.movements.fields.occurred_at_hint")}
      />
      <AppTextarea
        name="note"
        label={t("products.movements.fields.note")}
        className="sm:col-span-2"
      />
      {kind === "sale" && canCollect ? (
        <CollectionFields branchUuid={branchUuid} />
      ) : null}
    </>
  );
}

/**
 * Money taken with the sale (TEC-992): method, the till or bank account of
 * the branch and an optional amount (empty = the sale total). The accounts
 * come from the branch; without one the sale is recorded unpaid.
 */
function CollectionFields({ branchUuid }: { branchUuid: string }) {
  const { t } = useLocale();
  const form = useFormContext<MovementFormValues>();
  const [enabled, account] = useWatch({
    control: form.control,
    name: ["collection.enabled", "collection.cash_account_uuid"],
  });
  const accountsQuery = useStockPaymentAccounts(branchUuid || null);
  const accounts = accountsQuery.data?.collect_accounts;
  const options = useMemo(
    () => accountOptions(accounts ?? [], t),
    [accounts, t],
  );

  // Default to the first account of the branch; drop one of another branch.
  useEffect(() => {
    if (!accounts) return;
    if (!accounts.some((a) => a.uuid === account)) {
      form.setValue("collection.cash_account_uuid", accounts[0]?.uuid ?? "");
    }
  }, [account, accounts, form]);

  return (
    <div className="grid gap-4 sm:col-span-2">
      <Separator />
      <AppSwitch
        name="collection.enabled"
        label={t("products.movements.collection.toggle")}
        description={t("products.movements.collection.toggle_hint")}
      />
      {enabled ? (
        accounts && accounts.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t("products.movements.collection.no_accounts")}
            </AlertDescription>
          </Alert>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            <AppToggleGroup
              name="collection.method"
              label={t("products.movements.collection.method")}
              options={COLLECT_METHODS.map((method) => ({
                value: method,
                label: t(`products.movements.methods.${method}`),
              }))}
            />
            <AppSelect
              name="collection.cash_account_uuid"
              label={t("products.movements.collection.account")}
              placeholder={t(
                "products.movements.collection.account_placeholder",
              )}
              options={options}
              disabled={accountsQuery.isLoading}
            />
            <AppInput
              name="collection.amount"
              inputMode="decimal"
              label={t("products.movements.collection.amount")}
              description={t("products.movements.collection.amount_hint")}
            />
            <AppInput
              name="collection.note"
              label={t("products.movements.collection.note")}
            />
          </div>
        )
      ) : null}
    </div>
  );
}

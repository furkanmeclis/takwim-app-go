"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Minus, Plus, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { EntityRowActions, EntityTable } from "@/components/entity";
import { FormSection } from "@/components/forms";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { reservationsService } from "@/features/reservations/services/reservations.service";
import type {
  ReservationCreateRequest,
  ReservationProductOption,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { amount } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Most units of one product a visit takes (API limit). */
const MAX_QUANTITY = 99;
/** Most product lines of a visit (API limit). */
const MAX_PRODUCTS = 20;

/** A stock item picked in the new reservation form, with its quantity. */
export type BookingProduct = {
  key: string;
  option: ReservationProductOption;
  quantity: number;
};

/** Server messages of the picked products, by line index. */
export type BookingProductErrors = Record<number, string>;

type Row = BookingProduct & { index: number };

/** Combobox value of a stock item (product, or product + variant). */
function optionKey(o: ReservationProductOption) {
  return o.variant_uuid
    ? `${o.product_uuid}:${o.variant_uuid}`
    : o.product_uuid;
}

function optionName(o: ReservationProductOption) {
  return o.variant_name
    ? `${o.product_name} / ${o.variant_name}`
    : o.product_name;
}

/** Units that can still be picked: the available stock, at most 99. */
function maxOf(o: ReservationProductOption) {
  return Math.max(0, Math.min(o.available, MAX_QUANTITY));
}

/**
 * The products section is offered when the business has the products
 * module and the member may sell products (`tenant.products.sell`).
 */
export function useBookingProductsEnabled() {
  const { can } = usePermission();
  const hasModule = useNavModules();
  return hasModule("products") && can(permissions.tenant.products.sell);
}

/** The `products` of the create request (none: left out). */
export function toProductInputs(
  products: BookingProduct[],
): ReservationCreateRequest["products"] {
  if (!products.length) return undefined;
  return products.map((p) => ({
    product_uuid: p.option.product_uuid,
    variant_uuid: p.option.variant_uuid ?? undefined,
    quantity: p.quantity,
  }));
}

const DETAIL_KEYS: Record<string, string> = {
  insufficient_stock: "reservations.errors.insufficient_stock",
  inactive: "reservations.form_products.errors.inactive",
  not_offered: "reservations.form_products.errors.not_offered",
  not_found: "reservations.form_products.errors.not_found",
};

/**
 * Reads `products[i].field` details of a refused booking into messages per
 * line (null when the error names no product).
 */
export function productErrorsOf(
  error: unknown,
  t: (key: string) => string,
): BookingProductErrors | null {
  if (!isApiError(error)) return null;
  const out: BookingProductErrors = {};
  for (const d of error.details) {
    const m = /^products\[(\d+)\]\./.exec(d.field ?? "");
    if (!m) continue;
    const key = d.code ? DETAIL_KEYS[d.code] : undefined;
    out[Number(m[1])] = key ? t(key) : (d.message ?? "");
  }
  return Object.keys(out).length ? out : null;
}

/**
 * Optional products of a panel booking (TEC-977): stock items of the
 * branch with their sellable units (on hand − reserved). They are reserved
 * with the booking in the same transaction; a refused line is marked here.
 */
export function ReservationCreateProducts({
  id,
  branchUuid,
  value,
  onChange,
  errors,
}: {
  id: string;
  branchUuid: string;
  value: BookingProduct[];
  onChange: (next: BookingProduct[]) => void;
  errors: BookingProductErrors;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const currency = organization?.currency || "TRY";
  const [items, setItems] = useState<Record<string, ReservationProductOption>>(
    {},
  );
  const [pick, setPick] = useState("");

  // Another branch sells from another stock: start the products over.
  const lastBranch = useRef(branchUuid);
  useEffect(() => {
    if (lastBranch.current === branchUuid) return;
    lastBranch.current = branchUuid;
    setItems({});
    onChange([]);
  }, [branchUuid, onChange]);

  const toOption = useCallback(
    (o: ReservationProductOption): ComboboxOption => ({
      value: optionKey(o),
      label: optionName(o),
      description: t("reservations.products.option", {
        sku: o.sku,
        price: amount(o.price, currency, locale),
        available: o.available,
      }),
      disabled: o.available < 1,
    }),
    [currency, locale, t],
  );

  const loadOptions = useCallback(
    async (query: string) => {
      if (!branchUuid) return [];
      const result = await reservationsService.branchProductOptions(
        slug,
        branchUuid,
        query.trim() || undefined,
      );
      setItems((prev) => {
        const next = { ...prev };
        for (const o of result.items) next[optionKey(o)] = o;
        return next;
      });
      return result.items.map(toOption);
    },
    [branchUuid, slug, toOption],
  );

  const cached = useMemo(
    () => Object.values(items).map(toOption),
    [items, toOption],
  );

  const setQuantity = useCallback(
    (key: string, quantity: number) =>
      onChange(
        value
          .map((p) => (p.key === key ? { ...p, quantity } : p))
          .filter((p) => p.quantity > 0),
      ),
    [onChange, value],
  );

  // Picking an item adds one unit; picking it again adds another.
  const add = (key: string) => {
    setPick("");
    const option = items[key];
    if (!option) return;
    const current = value.find((p) => p.key === key);
    if (current) {
      setQuantity(key, Math.min(current.quantity + 1, maxOf(option)));
      return;
    }
    if (maxOf(option) < 1 || value.length >= MAX_PRODUCTS) return;
    onChange([...value, { key, option, quantity: 1 }]);
  };

  const rows = useMemo<Row[]>(
    () => value.map((p, index) => ({ ...p, index })),
    [value],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Row>({
          id: "product",
          accessorFn: (row) => optionName(row.option),
          labelKey: "reservations.products.columns.product",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="truncate font-medium">
                {optionName(row.original.option)}
              </p>
              <p className="text-muted-foreground truncate text-xs">
                {t("reservations.form_products.available", {
                  sku: row.original.option.sku,
                  available: row.original.option.available,
                })}
              </p>
              {errors[row.original.index] ? (
                <p className="text-destructive text-xs">
                  {errors[row.original.index]}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<Row>({
          accessorKey: "quantity",
          labelKey: "reservations.products.columns.quantity",
          enableSorting: false,
          cell: ({ row }) => {
            const p = row.original;
            return (
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="size-7"
                  aria-label={t("reservations.form_products.decrease")}
                  disabled={p.quantity <= 1}
                  onClick={() => setQuantity(p.key, p.quantity - 1)}
                >
                  <Minus className="size-3" />
                </Button>
                <span className="w-8 text-center tabular-nums">
                  {p.quantity}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="size-7"
                  aria-label={t("reservations.form_products.increase")}
                  disabled={p.quantity >= maxOf(p.option)}
                  onClick={() => setQuantity(p.key, p.quantity + 1)}
                >
                  <Plus className="size-3" />
                </Button>
              </div>
            );
          },
        }),
        createColumn<Row>({
          id: "unit_price",
          accessorFn: (row) => row.option.price,
          labelKey: "reservations.products.columns.unit_price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.option.price, currency, locale)}
            </span>
          ),
        }),
        createColumn<Row>({
          id: "total",
          accessorFn: (row) => Number(row.option.price) * row.quantity,
          labelKey: "reservations.products.columns.total",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(
                (
                  Number(row.original.option.price) * row.original.quantity
                ).toFixed(2),
                currency,
                locale,
              )}
            </span>
          ),
        }),
        createColumn<Row>({
          id: "actions",
          labelKey: "reservations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "remove",
                  label: t("reservations.form_products.remove"),
                  icon: Trash2,
                  variant: "destructive",
                  onSelect: () => setQuantity(row.original.key, 0),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Row, unknown>[],
    [currency, errors, locale, setQuantity, t],
  );

  const total = value.reduce(
    (sum, p) => sum + Number(p.option.price) * p.quantity,
    0,
  );

  return (
    <FormSection
      id={id}
      title={t("reservations.form.section_products")}
      description={t("reservations.form.section_products_desc")}
    >
      <div className="space-y-3">
        <div className="space-y-2">
          <Label htmlFor="booking-product-pick">
            {t("reservations.products.fields.product")}
          </Label>
          <AsyncCombobox
            id="booking-product-pick"
            // A new branch is a new stock: search it afresh.
            key={branchUuid}
            value={pick}
            onValueChange={add}
            loadOptions={loadOptions}
            options={cached}
            placeholder={t("reservations.form_products.placeholder")}
            searchPlaceholder={t("reservations.products.fields.search")}
            emptyText={t("reservations.products.fields.empty")}
            disabled={!branchUuid || value.length >= MAX_PRODUCTS}
          />
          <p className="text-muted-foreground text-sm">
            {branchUuid
              ? t("reservations.products.fields.product_hint")
              : t("reservations.form_products.branch_first")}
          </p>
        </div>
        {value.length > 0 ? (
          <>
            <EntityTable
              columns={columns}
              data={rows}
              getRowId={(row) => row.key}
              emptyTitle={t("reservations.products.empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
                globalFilter: false,
                sorting: false,
                pagination: false,
                persistKey: "tenant-reservation-create-products-v1",
              }}
            />
            <p className="text-muted-foreground text-sm">
              {t("reservations.form_products.summary", {
                total: amount(total.toFixed(2), currency, locale),
              })}
            </p>
          </>
        ) : null}
      </div>
    </FormSection>
  );
}

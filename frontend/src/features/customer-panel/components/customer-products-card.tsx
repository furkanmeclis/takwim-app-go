"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import type { CustomerReservation } from "@/features/customer-panel/types";
import { amount } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type CustomerProduct = CustomerReservation["products"][number];

const PRODUCT_STATUS = {
  held: { key: "customer_panel.products.status.held", tone: "warning" },
  delivered: {
    key: "customer_panel.products.status.delivered",
    tone: "success",
  },
  released: {
    key: "customer_panel.products.status.released",
    tone: "default",
  },
} as const;

/** "Şampuan / 500 ml". */
function productName(p: CustomerProduct) {
  return p.variant_name ? `${p.name} / ${p.variant_name}` : p.name;
}

/**
 * Products taken with the visit (TEC-977): reserved for the visit, handed
 * over at the visit, or released when the visit was cancelled. Hidden when
 * the visit has none.
 */
export function CustomerProductsCard({
  reservation,
}: {
  reservation: CustomerReservation;
}) {
  const { t, locale } = useLocale();
  const currency = reservation.money.currency;

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerProduct>({
          id: "name",
          accessorFn: (row) => productName(row),
          labelKey: "customer_panel.products.columns.product",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{productName(row.original)}</span>
          ),
        }),
        createColumn<CustomerProduct>({
          accessorKey: "quantity",
          labelKey: "customer_panel.products.columns.quantity",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.quantity}</span>
          ),
        }),
        createColumn<CustomerProduct>({
          accessorKey: "unit_price",
          labelKey: "customer_panel.products.columns.unit_price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.unit_price, currency, locale)}
            </span>
          ),
        }),
        createColumn<CustomerProduct>({
          accessorKey: "total",
          labelKey: "customer_panel.products.columns.total",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.total, currency, locale)}
            </span>
          ),
        }),
        createColumn<CustomerProduct>({
          accessorKey: "status",
          labelKey: "customer_panel.products.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(PRODUCT_STATUS[row.original.status].key)}
              tone={PRODUCT_STATUS[row.original.status].tone}
            />
          ),
        }),
      ] as ColumnDef<CustomerProduct, unknown>[],
    [currency, locale, t],
  );

  if (!reservation.products.length) return null;
  const released = reservation.products.every((p) => p.status === "released");

  return (
    <EntitySectionCard
      title={t("customer_panel.products.title")}
      badge={reservation.products.length}
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t(
          released
            ? "customer_panel.products.released_hint"
            : "customer_panel.products.hint",
        )}
      </p>
      <EntityTable
        columns={columns}
        data={reservation.products}
        getRowId={(row) => row.uuid}
        emptyTitle={t("customer_panel.products.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "customer-panel-products-v1",
        }}
      />
    </EntitySectionCard>
  );
}

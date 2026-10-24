"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import type { CustomerReservation } from "@/features/customer-panel/types";
import { amount, date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type CustomerPayment = CustomerReservation["payments"][number];

const KIND = {
  payment: { key: "customer_panel.ledger.kind.payment", tone: "success" },
  refund: { key: "customer_panel.ledger.kind.refund", tone: "default" },
} as const;

const METHOD_KEYS: Record<CustomerPayment["method"], string | null> = {
  "": null,
  cash: "customer_panel.ledger.method.cash",
  card: "customer_panel.ledger.method.card",
  transfer: "customer_panel.ledger.method.transfer",
  online: "customer_panel.ledger.method.online",
  other: "customer_panel.ledger.method.other",
};

/**
 * The visit's payments and refunds as the business recorded them in its
 * payment ledger, one row per partial payment (TEC-967).
 */
export function CustomerPaymentsCard({
  reservation,
}: {
  reservation: CustomerReservation;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<CustomerPayment>({
          accessorKey: "date",
          labelKey: "customer_panel.ledger.columns.date",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {date(row.original.date, undefined, locale)}
            </span>
          ),
        }),
        createColumn<CustomerPayment>({
          accessorKey: "kind",
          labelKey: "customer_panel.ledger.columns.kind",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(KIND[row.original.kind].key)}
              tone={KIND[row.original.kind].tone}
            />
          ),
        }),
        createColumn<CustomerPayment>({
          accessorKey: "method",
          labelKey: "customer_panel.ledger.columns.method",
          enableSorting: false,
          cell: ({ row }) => {
            const key = METHOD_KEYS[row.original.method];
            return key ? t(key) : "—";
          },
        }),
        createColumn<CustomerPayment>({
          accessorKey: "amount",
          labelKey: "customer_panel.ledger.columns.amount",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {row.original.kind === "refund" ? "−" : ""}
              {amount(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<CustomerPayment, unknown>[],
    [locale, t],
  );

  if (!reservation.payments.length) return null;

  return (
    <EntitySectionCard
      title={t("customer_panel.ledger.title")}
      badge={reservation.payments.length}
    >
      <div className="space-y-3">
        <p className="text-muted-foreground text-sm">
          {t("customer_panel.ledger.hint")}
        </p>
        <EntityTable
          columns={columns}
          data={reservation.payments}
          getRowId={(row) => row.uuid}
          emptyTitle={t("customer_panel.ledger.empty")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
            globalFilter: false,
            sorting: false,
            pagination: false,
            persistKey: "customer-panel-ledger-v1",
          }}
        />
      </div>
    </EntitySectionCard>
  );
}

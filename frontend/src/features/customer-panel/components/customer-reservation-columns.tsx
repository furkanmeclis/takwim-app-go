"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { createColumn } from "@/components/tables";
import { STATUS } from "@/features/reservations/constants";
import {
  PaymentChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import type { CustomerReservationItem } from "@/features/customer-panel/types";
import { amount, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export const METHOD_KEYS: Record<
  CustomerReservationItem["money"]["method"],
  string
> = {
  on_site: "customer_panel.method.on_site",
  online: "customer_panel.method.online",
};

type Options = {
  /** Payments view: paid / refunded / balance columns. */
  money?: boolean;
};

/**
 * Columns of the customer's visits across businesses. Times show in the
 * branch's own time zone, as the business plans them.
 */
export function useCustomerReservationColumns({ money = false }: Options = {}) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const cash = (value: string | null, currency: string) =>
      value == null ? "—" : amount(value, currency, locale);
    const columns = [
      createColumn<CustomerReservationItem>({
        accessorKey: "starts_at",
        labelKey: "customer_panel.columns.starts_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {datetimeInZone(
              row.original.starts_at,
              row.original.timezone,
              locale,
              {
                dateStyle: "medium",
                timeStyle: "short",
              },
            )}
          </span>
        ),
      }),
      createColumn<CustomerReservationItem>({
        id: "business",
        accessorFn: (row) => row.business.name,
        labelKey: "customer_panel.columns.business",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="font-medium">{row.original.business.name}</span>
            <span className="text-muted-foreground text-xs">
              {row.original.branch.name}
            </span>
          </div>
        ),
      }),
      createColumn<CustomerReservationItem>({
        id: "services",
        accessorFn: (row) => row.services.join(", "),
        labelKey: "customer_panel.columns.services",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="line-clamp-2">
            {row.original.services.join(", ") || "—"}
          </span>
        ),
      }),
      createColumn<CustomerReservationItem>({
        id: "staff",
        accessorFn: (row) => row.staff.join(", "),
        labelKey: "customer_panel.columns.staff",
        enableSorting: false,
        defaultHidden: money,
        cell: ({ row }) => row.original.staff.join(", ") || "—",
      }),
      createColumn<CustomerReservationItem>({
        accessorKey: "status",
        labelKey: "customer_panel.columns.status",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (Object.keys(STATUS) as (keyof typeof STATUS)[]).map(
          (value) => ({
            value,
            labelKey: STATUS[value].key,
            label: t(STATUS[value].key),
          }),
        ),
        cell: ({ row }) => (
          <ReservationStatusChip status={row.original.status} />
        ),
      }),
      createColumn<CustomerReservationItem>({
        accessorKey: "payment_status",
        labelKey: "customer_panel.columns.payment_status",
        enableSorting: false,
        cell: ({ row }) => <PaymentChip status={row.original.payment_status} />,
      }),
      createColumn<CustomerReservationItem>({
        id: "total",
        accessorFn: (row) => row.money.total,
        labelKey: "customer_panel.columns.total",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {cash(row.original.money.total, row.original.money.currency)}
          </span>
        ),
      }),
    ];
    if (money) {
      columns.push(
        createColumn<CustomerReservationItem>({
          id: "paid",
          accessorFn: (row) => row.money.paid,
          labelKey: "customer_panel.columns.paid",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {cash(row.original.money.paid, row.original.money.currency)}
            </span>
          ),
        }),
        createColumn<CustomerReservationItem>({
          id: "refunded",
          accessorFn: (row) => row.money.refunded,
          labelKey: "customer_panel.columns.refunded",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {cash(row.original.money.refunded, row.original.money.currency)}
            </span>
          ),
        }),
        createColumn<CustomerReservationItem>({
          id: "balance",
          accessorFn: (row) => row.money.balance,
          labelKey: "customer_panel.columns.balance",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {cash(row.original.money.balance, row.original.money.currency)}
            </span>
          ),
        }),
        createColumn<CustomerReservationItem>({
          id: "method",
          accessorFn: (row) => row.money.method,
          labelKey: "customer_panel.columns.method",
          enableSorting: false,
          cell: ({ row }) => t(METHOD_KEYS[row.original.money.method]),
        }),
      );
    }
    return columns as ColumnDef<CustomerReservationItem, unknown>[];
  }, [locale, money, t]);
}

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import type { CustomerPackage } from "@/features/customer-panel/types";
import {
  CUSTOMER_PACKAGE_STATES,
  STATE_LABEL_KEYS,
  STATE_TONES,
} from "@/features/session-packages/constants";
import { amount, date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Columns of "Paketlerim" (TEC-984): the package and its business, the
 * sessions left, the last day and the state. On phones the grid view shows
 * the package with its business and the remaining sessions.
 */
export function useCustomerPackageColumns() {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<CustomerPackage>({
          accessorKey: "name",
          labelKey: "customer_panel.packages.columns.package",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.business.name} · {row.original.service_name}
              </span>
            </div>
          ),
        }),
        createColumn<CustomerPackage>({
          id: "remaining",
          accessorFn: (row) => row.remaining,
          labelKey: "customer_panel.packages.columns.remaining",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {t("customer_panel.packages.remaining_of", {
                remaining: row.original.remaining,
                total: row.original.total_sessions,
              })}
            </span>
          ),
        }),
        createColumn<CustomerPackage>({
          id: "used",
          accessorFn: (row) => row.used,
          labelKey: "customer_panel.packages.columns.used",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.held > 0
                ? t("customer_panel.packages.used_held", {
                    used: row.original.used,
                    held: row.original.held,
                  })
                : row.original.used}
            </span>
          ),
        }),
        createColumn<CustomerPackage>({
          accessorKey: "expires_on",
          labelKey: "customer_panel.packages.columns.expires_on",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.expires_on
              ? date(row.original.expires_on, undefined, locale)
              : t("session_packages.validity_none"),
        }),
        createColumn<CustomerPackage>({
          accessorKey: "state",
          labelKey: "customer_panel.packages.columns.state",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CUSTOMER_PACKAGE_STATES.map((value) => ({
            value,
            labelKey: STATE_LABEL_KEYS[value],
            label: t(STATE_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(STATE_LABEL_KEYS[row.original.state])}
              tone={STATE_TONES[row.original.state]}
            />
          ),
        }),
        createColumn<CustomerPackage>({
          accessorKey: "sold_on",
          labelKey: "customer_panel.packages.columns.sold_on",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => date(row.original.sold_on, undefined, locale),
        }),
        createColumn<CustomerPackage>({
          accessorKey: "price",
          labelKey: "customer_panel.packages.columns.price",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {amount(row.original.price, row.original.currency, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<CustomerPackage, unknown>[],
    [locale, t],
  );
}

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  ServiceRowActions,
  type ServiceRowActionHandlers,
} from "@/features/services/components/service-row-actions";
import {
  PAYMENT_MODE_LABEL_KEYS,
  STAFF_MODE_LABEL_KEYS,
  UNCATEGORIZED,
} from "@/features/services/constants";
import { priceLabel } from "@/features/services/lib/format";
import type {
  PaymentMode,
  Service,
  ServiceFacets,
} from "@/features/services/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type ServiceColumnsOptions = {
  handlers: ServiceRowActionHandlers;
  facets?: ServiceFacets;
};

/**
 * Columns follow `GET /v1/tenant/services/meta`: sortable keys match
 * `sortable_fields`; `category`, `branch`, `payment_mode`, `is_active` and
 * `show_online` are server-side faceted filters.
 */
export function useServiceColumns({ handlers, facets }: ServiceColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const yesNo = (yes: string, no: string) => [
      { value: "true", labelKey: yes, label: t(yes) },
      { value: "false", labelKey: no, label: t(no) },
    ];
    return [
      createColumn<Service>({
        accessorKey: "name",
        labelKey: "services.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<Service>({
        id: "category",
        accessorFn: (row) => row.category?.name ?? "",
        labelKey: "services.columns.category",
        enableSorting: false,
        gridSecondary: true,
        filterVariant: "faceted",
        filterOptions: (facets?.category ?? []).map((facet) => ({
          value: facet.value,
          label: `${
            facet.value === UNCATEGORIZED
              ? t("services.uncategorized")
              : (facet.label ?? facet.value)
          } (${facet.count})`,
        })),
        cell: ({ row }) =>
          row.original.category ? (
            <Badge variant="secondary">{row.original.category.name}</Badge>
          ) : (
            <span className="text-muted-foreground">
              {t("services.uncategorized")}
            </span>
          ),
      }),
      createColumn<Service>({
        id: "duration",
        accessorKey: "duration_minutes",
        labelKey: "services.columns.duration",
        enableSorting: true,
        cell: ({ row }) =>
          t("services.minutes", {
            minutes: String(row.original.duration_minutes),
          }),
      }),
      createColumn<Service>({
        accessorKey: "price",
        labelKey: "services.columns.price",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) =>
          priceLabel(row.original.price, row.original.currency, locale),
      }),
      createColumn<Service>({
        accessorKey: "payment_mode",
        labelKey: "services.columns.payment_mode",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.payment_mode ?? []).map((facet) => ({
          value: facet.value,
          label: `${t(PAYMENT_MODE_LABEL_KEYS[facet.value as PaymentMode] ?? facet.value)} (${facet.count})`,
        })),
        cell: ({ row }) =>
          t(PAYMENT_MODE_LABEL_KEYS[row.original.payment_mode]),
      }),
      createColumn<Service>({
        id: "branch",
        accessorFn: (row) => row.offers.length,
        labelKey: "services.columns.branches",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.label ?? facet.value} (${facet.count})`,
        })),
        cell: ({ row }) => {
          const offers = row.original.offers;
          if (offers.length === 0) {
            return (
              <span className="text-muted-foreground">
                {t("services.no_branches")}
              </span>
            );
          }
          return (
            <span className="line-clamp-1">
              {offers.map((offer) => offer.branch_code).join(", ")}
            </span>
          );
        },
      }),
      createColumn<Service>({
        accessorKey: "staff_mode",
        labelKey: "services.columns.staff_mode",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => t(STAFF_MODE_LABEL_KEYS[row.original.staff_mode]),
      }),
      createColumn<Service>({
        id: "show_online",
        accessorFn: (row) => String(row.show_online),
        labelKey: "services.columns.show_online",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "services.online.visible",
          "services.online.hidden",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.show_online
                ? t("services.online.visible")
                : t("services.online.hidden")
            }
            tone={row.original.show_online ? "success" : "default"}
          />
        ),
      }),
      createColumn<Service>({
        id: "is_active",
        accessorFn: (row) => String(row.is_active),
        labelKey: "services.columns.is_active",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "services.status.active",
          "services.status.inactive",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? t("services.status.active")
                : t("services.status.inactive")
            }
            tone={row.original.is_active ? "success" : "default"}
          />
        ),
      }),
      createColumn<Service>({
        accessorKey: "sort_order",
        labelKey: "services.columns.sort_order",
        enableSorting: true,
        defaultHidden: true,
      }),
      createColumn<Service>({
        accessorKey: "created_at",
        labelKey: "services.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<Service>({
        accessorKey: "uuid",
        labelKey: "services.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<Service>({
        id: "actions",
        labelKey: "services.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <ServiceRowActions service={row.original} handlers={handlers} />
        ),
      }),
    ] as ColumnDef<Service, unknown>[];
  }, [
    facets?.branch,
    facets?.category,
    facets?.payment_mode,
    handlers,
    locale,
    t,
  ]);
}

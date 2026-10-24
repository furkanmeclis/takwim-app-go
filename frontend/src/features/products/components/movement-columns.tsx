"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { HandCoins, Undo2, Wallet } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import { MOVEMENT_KIND_LABEL_KEYS } from "@/features/products/constants";
import {
  priceLabel,
  salePayment,
  signedQuantity,
} from "@/features/products/lib/format";
import type {
  MovementKind,
  StockMovement,
  StockMovementFacets,
} from "@/features/products/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const KIND_TONES: Record<MovementKind, "success" | "warning" | "default"> = {
  receipt: "success",
  return: "success",
  transfer_in: "success",
  sale: "default",
  transfer_out: "warning",
  adjustment: "warning",
};

/**
 * Ledger columns (`GET /v1/tenant/stock-movements/meta`): `occurred_at`,
 * `quantity` and `kind` sort on the server; `kind`, `branch` and `staff`
 * are faceted filters. `withProduct` drops the product column on a
 * product's own page. With `onReturn`, a sale not yet taken back offers
 * the return (the caller checks the stock and refund permissions); with
 * `onCollect` a sale not paid in full offers to take the money, with
 * `onRefund` a returned sale still holding money offers to pay it back
 * (TEC-992).
 */
export function useMovementColumns({
  facets,
  withProduct = true,
  filters = true,
  onReturn,
  onCollect,
  onRefund,
}: {
  facets?: StockMovementFacets;
  withProduct?: boolean;
  filters?: boolean;
  onReturn?: (movement: StockMovement) => void;
  onCollect?: (movement: StockMovement) => void;
  onRefund?: (movement: StockMovement) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();

  return useMemo(() => {
    const facetOptions = (
      list: StockMovementFacets["kind"] | undefined,
      label: (value: string, fallback?: string) => string,
    ) =>
      (list ?? []).map((facet) => ({
        value: facet.value,
        label: `${label(facet.value, facet.label ?? undefined)} (${facet.count})`,
      }));
    const columns = [
      createColumn<StockMovement>({
        accessorKey: "occurred_at",
        labelKey: "products.movements.columns.occurred_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) =>
          datetime(row.original.occurred_at, undefined, locale),
      }),
      createColumn<StockMovement>({
        accessorKey: "kind",
        labelKey: "products.movements.columns.kind",
        enableSorting: true,
        gridPrimary: !withProduct,
        ...(filters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: facetOptions(facets?.kind, (value) =>
                t(MOVEMENT_KIND_LABEL_KEYS[value as MovementKind] ?? value),
              ),
            }
          : {}),
        cell: ({ row }) => (
          <span className="flex flex-wrap items-center gap-1">
            <StatusChip
              label={t(MOVEMENT_KIND_LABEL_KEYS[row.original.kind])}
              tone={KIND_TONES[row.original.kind]}
            />
            {row.original.returned ? (
              <Badge variant="outline">
                {t("products.movements.returned")}
              </Badge>
            ) : null}
          </span>
        ),
      }),
      ...(withProduct
        ? [
            createColumn<StockMovement>({
              id: "product",
              accessorFn: (row) => row.product.name,
              labelKey: "products.movements.columns.product",
              enableSorting: false,
              gridPrimary: true,
              cell: ({ row }) => (
                <span className="flex flex-col">
                  <Link
                    href={routes.tenant.products.detail(
                      slug,
                      row.original.product.uuid,
                    )}
                    className="font-medium hover:underline"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {row.original.product.name}
                  </Link>
                  {row.original.variant ? (
                    <span className="text-muted-foreground text-xs">
                      {row.original.variant.name}
                    </span>
                  ) : null}
                </span>
              ),
            }),
          ]
        : [
            createColumn<StockMovement>({
              id: "variant",
              accessorFn: (row) => row.variant?.name ?? "",
              labelKey: "products.movements.columns.variant",
              enableSorting: false,
              cell: ({ row }) =>
                row.original.variant?.name ?? (
                  <span className="text-muted-foreground">—</span>
                ),
            }),
          ]),
      createColumn<StockMovement>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "products.movements.columns.branch",
        enableSorting: false,
        ...(filters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: facetOptions(
                facets?.branch,
                (value, fallback) => fallback ?? value,
              ),
            }
          : {}),
        cell: ({ row }) => (
          <span className="flex flex-col">
            <span>{row.original.branch.name}</span>
            {row.original.counterpart_branch ? (
              <span className="text-muted-foreground text-xs">
                {row.original.kind === "transfer_out"
                  ? t("products.movements.to_branch", {
                      branch: row.original.counterpart_branch.name,
                    })
                  : t("products.movements.from_branch", {
                      branch: row.original.counterpart_branch.name,
                    })}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<StockMovement>({
        accessorKey: "quantity",
        labelKey: "products.movements.columns.quantity",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span
            className={
              row.original.quantity < 0
                ? "text-destructive font-medium tabular-nums"
                : "font-medium tabular-nums"
            }
          >
            {signedQuantity(row.original.quantity, locale)}
          </span>
        ),
      }),
      createColumn<StockMovement>({
        accessorKey: "balance_after",
        labelKey: "products.movements.columns.balance_after",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {row.original.balance_after.toLocaleString(locale)}
          </span>
        ),
      }),
      createColumn<StockMovement>({
        id: "unit_price",
        accessorFn: (row) => row.unit_price ?? row.unit_cost ?? "",
        labelKey: "products.movements.columns.unit_price",
        enableSorting: false,
        defaultHidden: !withProduct,
        cell: ({ row }) => {
          const amount = row.original.unit_price ?? row.original.unit_cost;
          return amount ? (
            priceLabel(amount, row.original.currency, locale)
          ) : (
            <span className="text-muted-foreground">—</span>
          );
        },
      }),
      createColumn<StockMovement>({
        id: "payment",
        accessorFn: (row) => row.collected ?? "",
        labelKey: "products.movements.columns.payment",
        enableSorting: false,
        cell: ({ row }) => {
          const payment = salePayment(row.original);
          if (!payment) return <span className="text-muted-foreground">—</span>;
          return (
            <span className="flex flex-col gap-1">
              <StatusChip
                label={t(`products.movements.payment.${payment.state}`)}
                tone={payment.tone}
              />
              {payment.state !== "unpaid" ? (
                <span className="text-muted-foreground text-xs tabular-nums">
                  {priceLabel(payment.collected, row.original.currency, locale)}
                  {payment.refunded > 0
                    ? ` · ${t("products.movements.payment.refunded_amount", {
                        amount: priceLabel(
                          String(payment.refunded),
                          row.original.currency,
                          locale,
                        ),
                      })}`
                    : ""}
                </span>
              ) : null}
            </span>
          );
        },
      }),
      createColumn<StockMovement>({
        id: "staff",
        accessorFn: (row) => row.staff?.name ?? "",
        labelKey: "products.movements.columns.staff",
        enableSorting: false,
        defaultHidden: !withProduct,
        ...(filters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: facetOptions(
                facets?.staff,
                (value, fallback) => fallback ?? value,
              ),
            }
          : {}),
        cell: ({ row }) =>
          row.original.staff?.name ?? (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<StockMovement>({
        accessorKey: "reason",
        labelKey: "products.movements.columns.reason",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.reason ? (
            <span className="line-clamp-2">{row.original.reason}</span>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<StockMovement>({
        id: "created_by",
        accessorFn: (row) => row.created_by?.name ?? "",
        labelKey: "products.movements.columns.created_by",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.created_by?.name ?? (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<StockMovement>({
        accessorKey: "note",
        labelKey: "products.movements.columns.note",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="line-clamp-2">{row.original.note}</span>
        ),
      }),
      ...(onReturn || onCollect || onRefund
        ? [
            createColumn<StockMovement>({
              id: "actions",
              labelKey: "products.movements.columns.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) => {
                const movement = row.original;
                if (movement.kind !== "sale") return null;
                const payment = salePayment(movement);
                const actions = [
                  ...(onCollect &&
                  !movement.returned &&
                  payment &&
                  payment.remaining > 0
                    ? [
                        {
                          id: "collect",
                          label: t("products.movements.collect.action"),
                          icon: HandCoins,
                          onSelect: () => onCollect(movement),
                        },
                      ]
                    : []),
                  ...(onReturn && !movement.returned
                    ? [
                        {
                          id: "return",
                          label: t("products.movements.return.action"),
                          icon: Undo2,
                          onSelect: () => onReturn(movement),
                        },
                      ]
                    : []),
                  ...(onRefund &&
                  movement.returned &&
                  payment &&
                  payment.refundable > 0
                    ? [
                        {
                          id: "refund",
                          label: t("products.movements.refund.action"),
                          icon: Wallet,
                          onSelect: () => onRefund(movement),
                        },
                      ]
                    : []),
                ];
                return actions.length ? (
                  <EntityRowActions actions={actions} />
                ) : null;
              },
            }),
          ]
        : []),
    ];
    return columns as ColumnDef<StockMovement, unknown>[];
  }, [
    facets?.branch,
    facets?.kind,
    facets?.staff,
    filters,
    locale,
    onCollect,
    onRefund,
    onReturn,
    slug,
    t,
    withProduct,
  ]);
}

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { createColumn } from "@/components/tables";
import {
  APPROVAL,
  PAYMENT,
  SOURCE,
  STATUS,
} from "@/features/reservations/constants";
import {
  ApprovalChip,
  PaymentChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import {
  ReservationRowActions,
  type ReservationRowActionHandlers,
} from "@/features/reservations/components/reservation-row-actions";
import {
  partyLabel,
  sourceLabel,
  vehicleLabel,
} from "@/features/reservations/lib/labels";
import type {
  ReservationFacets,
  ReservationListItem,
} from "@/features/reservations/types";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { amount, datetime, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Options = {
  handlers: ReservationRowActionHandlers;
  facets?: ReservationFacets;
};

function enumOptions<T extends string>(
  map: Record<T, { key: string } | string>,
  t: (key: string) => string,
) {
  return (Object.keys(map) as T[]).map((value) => {
    const entry = map[value];
    const key = typeof entry === "string" ? entry : entry.key;
    return { value, labelKey: key, label: t(key) };
  });
}

/**
 * Columns follow `GET /v1/tenant/reservations/meta`: sortable keys match
 * `sortable_fields`; branch and staff are dynamic facets, the states are
 * static enums. Times are shown in the business's time zone.
 */
export function useReservationColumns({ handlers, facets }: Options) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  const { sector } = useSector();
  const party = Boolean(sector?.capacity.party_size);
  const vehicle = Boolean(sector?.flow.includes("vehicle"));

  return useMemo(() => {
    const facetOptions = (list?: ReservationFacets["branch"]) =>
      (list ?? []).map((facet) => ({
        value: facet.value,
        label: `${facet.label ?? facet.value} (${facet.count})`,
      }));
    return [
      createColumn<ReservationListItem>({
        accessorKey: "number",
        labelKey: "reservations.columns.number",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="font-medium tabular-nums">
            #{row.original.number}
          </span>
        ),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "starts_at",
        labelKey: "reservations.columns.starts_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {datetimeInZone(row.original.starts_at, zone, locale, {
              dateStyle: "medium",
              timeStyle: "short",
            })}
          </span>
        ),
      }),
      createColumn<ReservationListItem>({
        id: "customer",
        accessorFn: (row) => row.customer.name,
        labelKey: "reservations.columns.customer",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="font-medium">{row.original.customer.name}</span>
            {row.original.customer.phone ? (
              <span className="text-muted-foreground text-xs">
                {row.original.customer.phone}
              </span>
            ) : null}
          </div>
        ),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "services",
        labelKey: "reservations.columns.services",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="line-clamp-2">{row.original.services || "—"}</span>
        ),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "party_size",
        labelKey: "reservations.columns.party",
        enableSorting: false,
        // Shown where the sector books by party size (restaurants).
        defaultHidden: !party,
        cell: ({ row }) => partyLabel(t, row.original) || "—",
      }),
      createColumn<ReservationListItem>({
        accessorKey: "vehicle",
        labelKey: "reservations.columns.vehicle",
        enableSorting: false,
        // Shown where the sector asks for the vehicle (car washes).
        defaultHidden: !vehicle,
        cell: ({ row }) => vehicleLabel(t, row.original) || "—",
      }),
      createColumn<ReservationListItem>({
        accessorKey: "staff",
        labelKey: "reservations.columns.staff",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.staff),
        cell: ({ row }) => row.original.staff || "—",
      }),
      createColumn<ReservationListItem>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "reservations.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: facetOptions(facets?.branch),
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<ReservationListItem>({
        accessorKey: "status",
        labelKey: "reservations.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: enumOptions(STATUS, t),
        cell: ({ row }) => (
          <ReservationStatusChip status={row.original.status} />
        ),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "approval_status",
        labelKey: "reservations.columns.approval_status",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumOptions(APPROVAL, t),
        cell: ({ row }) => (
          <ApprovalChip status={row.original.approval_status} />
        ),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "payment_status",
        labelKey: "reservations.columns.payment_status",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumOptions(PAYMENT, t),
        cell: ({ row }) => <PaymentChip status={row.original.payment_status} />,
      }),
      createColumn<ReservationListItem>({
        accessorKey: "source",
        labelKey: "reservations.columns.source",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: enumOptions(SOURCE, t),
        cell: ({ row }) => sourceLabel(t, row.original.source),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "total_price",
        labelKey: "reservations.columns.total_price",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {amount(row.original.total_price, row.original.currency, locale)}
          </span>
        ),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "created_at",
        labelKey: "reservations.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<ReservationListItem>({
        accessorKey: "uuid",
        labelKey: "reservations.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<ReservationListItem>({
        id: "actions",
        labelKey: "reservations.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <ReservationRowActions
            reservation={row.original}
            handlers={handlers}
          />
        ),
      }),
    ] as ColumnDef<ReservationListItem, unknown>[];
  }, [
    facets?.branch,
    facets?.staff,
    handlers,
    locale,
    party,
    t,
    vehicle,
    zone,
  ]);
}

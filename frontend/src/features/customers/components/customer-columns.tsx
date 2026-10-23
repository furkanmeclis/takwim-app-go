"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { BadgeCheck } from "lucide-react";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  CustomerRowActions,
  type CustomerRowActionHandlers,
} from "@/features/customers/components/customer-row-actions";
import { sourceLabel } from "@/features/customers/lib/labels";
import type { Customer, CustomerFacets } from "@/features/customers/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type CustomerColumnsOptions = {
  handlers: CustomerRowActionHandlers;
  facets?: CustomerFacets;
};

/**
 * Columns follow `GET /v1/tenant/customers/meta`: sortable keys match
 * `sortable_fields`; `tags`, `source`, `verified` and the consents are
 * server-side faceted filters.
 */
export function useCustomerColumns({
  handlers,
  facets,
}: CustomerColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const yesNo = (yes: string, no: string) => [
      { value: "true", labelKey: yes, label: t(yes) },
      { value: "false", labelKey: no, label: t(no) },
    ];
    const consent = (value: boolean) => (
      <StatusChip
        label={
          value ? t("customers.consent.given") : t("customers.consent.none")
        }
        tone={value ? "success" : "default"}
      />
    );
    return [
      createColumn<Customer>({
        id: "name",
        accessorFn: (row) => row.full_name,
        labelKey: "customers.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex items-center gap-1.5 font-medium">
            {row.original.full_name}
            {row.original.verification ? (
              <BadgeCheck
                className="text-primary size-4 shrink-0"
                aria-label={t("customers.verified.yes")}
              />
            ) : null}
          </span>
        ),
      }),
      createColumn<Customer>({
        id: "phone",
        accessorFn: (row) => row.phone_display,
        labelKey: "customers.columns.phone",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => row.original.phone_display || "—",
      }),
      createColumn<Customer>({
        accessorKey: "email",
        labelKey: "customers.columns.email",
        enableSorting: true,
        cell: ({ row }) => row.original.email || "—",
      }),
      createColumn<Customer>({
        id: "tags",
        accessorFn: (row) => row.tags.join(", "),
        labelKey: "customers.columns.tags",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.tag ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.value} (${facet.count})`,
        })),
        cell: ({ row }) =>
          row.original.tags.length ? (
            <div className="flex flex-wrap gap-1">
              {row.original.tags.map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                </Badge>
              ))}
            </div>
          ) : (
            "—"
          ),
      }),
      createColumn<Customer>({
        id: "verified",
        accessorFn: (row) => String(Boolean(row.verification)),
        labelKey: "customers.columns.verified",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: yesNo("customers.verified.yes", "customers.verified.no"),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.verification
                ? t("customers.verified.yes")
                : t("customers.verified.no")
            }
            tone={row.original.verification ? "success" : "default"}
          />
        ),
      }),
      createColumn<Customer>({
        accessorKey: "source",
        labelKey: "customers.columns.source",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: (facets?.source ?? []).map((facet) => ({
          value: facet.value,
          label: `${sourceLabel(t, facet.value)} (${facet.count})`,
        })),
        cell: ({ row }) => sourceLabel(t, row.original.source),
      }),
      createColumn<Customer>({
        id: "sms_consent",
        accessorFn: (row) => String(row.sms_consent),
        labelKey: "customers.columns.sms_consent",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "customers.consent.given",
          "customers.consent.none",
        ),
        cell: ({ row }) => consent(row.original.sms_consent),
      }),
      createColumn<Customer>({
        id: "email_consent",
        accessorFn: (row) => String(row.email_consent),
        labelKey: "customers.columns.email_consent",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "customers.consent.given",
          "customers.consent.none",
        ),
        cell: ({ row }) => consent(row.original.email_consent),
      }),
      createColumn<Customer>({
        accessorKey: "created_at",
        labelKey: "customers.columns.created_at",
        enableSorting: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<Customer>({
        accessorKey: "uuid",
        labelKey: "customers.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<Customer>({
        id: "actions",
        labelKey: "customers.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <CustomerRowActions customer={row.original} handlers={handlers} />
        ),
      }),
    ] as ColumnDef<Customer, unknown>[];
  }, [facets?.source, facets?.tag, handlers, locale, t]);
}

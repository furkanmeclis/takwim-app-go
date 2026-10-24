"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Archive } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  type CloseTarget,
  CloseWithoutRefundDialog,
} from "@/features/reservations/components/reservation-refunds";
import {
  PAYMENT,
  RESERVATIONS_RESOURCE,
  STATUS,
  type Tone,
} from "@/features/reservations/constants";
import {
  usePaymentFollowUpFacets,
  usePaymentFollowUps,
} from "@/features/reservations/hooks/use-reservations";
import { FOLLOW_UP_ANY } from "@/features/reservations/lib/permissions";
import type {
  ListPaymentFollowUpsParams,
  PaymentFollowUp,
  PaymentFollowUpKind,
  ReservationStatus,
} from "@/features/reservations/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { amount, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Follow-up kinds, in the order a manager handles them. */
const FOLLOW_UP_KINDS: Record<
  PaymentFollowUpKind,
  { key: string; tone: Tone }
> = {
  refund_unverified: {
    key: "reservations.follow_ups.kinds.refund_unverified",
    tone: "warning",
  },
  refund_failed: {
    key: "reservations.follow_ups.kinds.refund_failed",
    tone: "danger",
  },
  refund_pending: {
    key: "reservations.follow_ups.kinds.refund_pending",
    tone: "warning",
  },
  unpaid: { key: "reservations.follow_ups.kinds.unpaid", tone: "danger" },
};

/** Kinds a manager may close without paying the money back. */
const CLOSABLE_KINDS: PaymentFollowUpKind[] = [
  "refund_pending",
  "refund_failed",
];

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * Visits whose money needs a person: paid and then cancelled (or paid
 * online after the cancel), a refund whose provider result is unknown or
 * that failed, and completed visits not paid in full. A row opens the
 * visit, where the refund or the payment is recorded; a refund to make can
 * also be closed here without paying it back.
 */
export function PaymentFollowUpsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const canClose = can(permissions.tenant.payment.refund);
  const [closing, setClosing] = useState<CloseTarget | null>(null);

  // A refund, a payment or a cancel publishes on the reservations channel.
  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RESERVATIONS_RESOURCE),
  });

  const listParams = useMemo<ListPaymentFollowUpsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      kind: joined(column("kind")),
      branch: joined(column("branch")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = usePaymentFollowUps(listParams);
  const facetsQuery = usePaymentFollowUpFacets({
    q: listParams.q,
    branch: listParams.branch,
  });
  const branchesQuery = useAccessibleBranches();

  const openDetail = useCallback(
    (row: PaymentFollowUp) =>
      router.push(routes.tenant.reservations.detail(slug, row.uuid)),
    [router, slug],
  );

  const kindCounts = facetsQuery.data?.kind;
  const branches = branchesQuery.data?.items;
  const columns = useMemo(() => {
    const kindOptions = (
      Object.keys(FOLLOW_UP_KINDS) as PaymentFollowUpKind[]
    ).map((kind) => {
      const count = kindCounts?.find((facet) => facet.value === kind)?.count;
      return {
        value: kind,
        label: `${t(FOLLOW_UP_KINDS[kind].key)} (${count ?? 0})`,
      };
    });
    const branchOptions = (branches ?? []).map((branch) => ({
      value: branch.uuid,
      label: branch.name,
    }));
    const money = (row: PaymentFollowUp, value: string) => (
      <span className="tabular-nums">
        {amount(value, row.currency, locale)}
      </span>
    );
    return [
      createColumn<PaymentFollowUp>({
        accessorKey: "number",
        labelKey: "reservations.follow_ups.columns.number",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex flex-col">
            <Link
              href={routes.tenant.reservations.detail(slug, row.original.uuid)}
              className="font-medium hover:underline"
              onClick={(event) => event.stopPropagation()}
            >
              #{row.original.number}
            </Link>
            <span className="text-muted-foreground text-xs">
              {row.original.customer?.name ??
                t("reservations.follow_ups.no_customer")}
            </span>
          </span>
        ),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "kind",
        labelKey: "reservations.follow_ups.columns.kind",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: kindOptions,
        cell: ({ row }) => (
          <span className="flex flex-wrap items-center gap-1">
            <StatusChip
              label={t(FOLLOW_UP_KINDS[row.original.kind].key)}
              tone={FOLLOW_UP_KINDS[row.original.kind].tone}
            />
            {row.original.late ? (
              <Badge variant="outline">
                {t("reservations.follow_ups.late")}
              </Badge>
            ) : null}
          </span>
        ),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "amount",
        labelKey: "reservations.follow_ups.columns.amount",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-medium">
            {money(row.original, row.original.amount)}
          </span>
        ),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "starts_at",
        labelKey: "reservations.follow_ups.columns.starts_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {datetime(row.original.starts_at, undefined, locale)}
          </span>
        ),
      }),
      createColumn<PaymentFollowUp>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "reservations.follow_ups.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: branchOptions,
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "status",
        labelKey: "reservations.follow_ups.columns.status",
        enableSorting: false,
        cell: ({ row }) => {
          const status = STATUS[row.original.status as ReservationStatus];
          return status ? (
            <StatusChip label={t(status.key)} tone={status.tone} />
          ) : (
            row.original.status
          );
        },
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "payment_status",
        labelKey: "reservations.follow_ups.columns.payment_status",
        enableSorting: false,
        cell: ({ row }) => (
          <StatusChip
            label={t(PAYMENT[row.original.payment_status].key)}
            tone={PAYMENT[row.original.payment_status].tone}
          />
        ),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "charged",
        labelKey: "reservations.follow_ups.columns.charged",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => money(row.original, row.original.charged),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "collected",
        labelKey: "reservations.follow_ups.columns.collected",
        enableSorting: false,
        cell: ({ row }) => money(row.original, row.original.collected),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "refunded",
        labelKey: "reservations.follow_ups.columns.refunded",
        enableSorting: false,
        cell: ({ row }) => money(row.original, row.original.refunded),
      }),
      createColumn<PaymentFollowUp>({
        id: "last_refund",
        accessorFn: (row) => row.last_refund_failure,
        labelKey: "reservations.follow_ups.columns.last_refund",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.last_refund_status ? (
            <span className="flex flex-col">
              <span>
                {t(
                  `reservations.refunds.status.${row.original.last_refund_status}`,
                )}
              </span>
              {row.original.last_refund_failure ? (
                <span className="text-muted-foreground line-clamp-2 text-xs">
                  {row.original.last_refund_failure}
                </span>
              ) : null}
            </span>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<PaymentFollowUp>({
        accessorKey: "updated_at",
        labelKey: "reservations.follow_ups.columns.updated_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {datetime(row.original.updated_at, undefined, locale)}
          </span>
        ),
      }),
      createColumn<PaymentFollowUp>({
        id: "actions",
        labelKey: "reservations.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          canClose && CLOSABLE_KINDS.includes(row.original.kind) ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={(event) => {
                event.stopPropagation();
                setClosing({
                  uuid: row.original.uuid,
                  number: row.original.number,
                  amount: row.original.amount,
                  currency: row.original.currency,
                });
              }}
            >
              <Archive className="size-4" />
              {t("reservations.refunds.close")}
            </Button>
          ) : null,
      }),
    ] as ColumnDef<PaymentFollowUp, unknown>[];
  }, [branches, canClose, kindCounts, locale, slug, t]);

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.kind) ||
    Boolean(listParams.branch);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("reservations.follow_ups.title")}
      description={t("reservations.follow_ups.description")}
      permission={FOLLOW_UP_ANY}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.follow_ups.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("reservations.follow_ups.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("reservations.follow_ups.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("reservations.follow_ups.empty_filtered_title")
            : t("reservations.follow_ups.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("reservations.follow_ups.empty_filtered_description")
            : t("reservations.follow_ups.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-payment-follow-ups-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={refresh}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <CloseWithoutRefundDialog
        target={closing}
        onOpenChange={(open) => {
          if (!open) setClosing(null);
        }}
      />
    </EntityPage>
  );
}

"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import {
  EntityDetail,
  EntityDrawer,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { routes } from "@/config/routes";
import { useCashAmount } from "@/features/cash/lib/format";
import {
  LedgerEntryKindBadges,
  useLedgerEntryColumns,
} from "@/features/sales/components/ledger-entry-columns";
import {
  DIRECTION_LABEL_KEYS,
  LEDGER_ENTRIES_RESOURCE,
  METHOD_LABEL_KEYS,
} from "@/features/sales/constants";
import { useLedgerEntries } from "@/features/sales/hooks/use-sales";
import { useSalesFilterOptions } from "@/features/sales/hooks/use-sales-filter-options";
import { signedEntryAmount } from "@/features/sales/lib/format";
import type {
  LedgerEntry,
  ListLedgerEntriesParams,
  SalesPeriod,
} from "@/features/sales/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FILTER_IDS = ["kind", "branch"] as const;
const SORT_FIELDS = ["entry_date", "amount", "kind", "created_at"] as const;

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * The customer ledger of the period: charges (sale, package), collections,
 * refunds and reversals, with server-side filters and live updates. A row
 * opens a read-only drawer; payments and refunds are taken on the visit.
 */
export function LedgerEntriesTable({ period }: { period: SalesPeriod }) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-entry_date",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 50,
    realtime: tenantLiveList(organization, LEDGER_ENTRIES_RESOURCE),
  });

  const params = useMemo<ListLedgerEntriesParams>(() => {
    const column = (id: string) =>
      joined(
        filterValues(
          listState.columnFilters.find((filter) => filter.id === id)?.value,
        ),
      );
    return {
      ...listState.params,
      kind: column("kind"),
      branch: column("branch"),
      date_from: period.date_from,
      date_to: period.date_to,
    };
  }, [listState.columnFilters, listState.params, period]);

  const query = useLedgerEntries(params);
  const { branchOptions } = useSalesFilterOptions();
  const columns = useLedgerEntryColumns({ branchOptions });
  const [viewing, setViewing] = useState<LedgerEntry | null>(null);

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 50)),
  );
  const hasFilters = Boolean(params.q || params.kind || params.branch);

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("sales.entries.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        onRowClick={setViewing}
        emptyTitle={
          hasFilters
            ? t("sales.empty_filtered_title")
            : t("sales.entries.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("sales.empty_filtered_description")
            : t("sales.entries.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-ledger-entries-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <LedgerEntryDrawer
        entry={viewing}
        onOpenChange={(open) => {
          if (!open) setViewing(null);
        }}
      />
    </>
  );
}

function LedgerEntryDrawer({
  entry,
  onOpenChange,
}: {
  entry: LedgerEntry | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const money = useCashAmount();

  return (
    <EntityDrawer
      open={Boolean(entry)}
      onOpenChange={onOpenChange}
      title={t("sales.entry_drawer.title")}
      description={t("sales.entry_drawer.description")}
    >
      {entry ? (
        <EntityDetail
          sections={[
            {
              id: "entry",
              fields: [
                {
                  key: "kind",
                  label: t("sales.entries.columns.kind"),
                  value: <LedgerEntryKindBadges entry={entry} />,
                },
                {
                  key: "amount",
                  label: t("sales.entries.columns.amount"),
                  value: (
                    <span className="font-semibold tabular-nums">
                      {money(signedEntryAmount(entry), entry.currency)}
                    </span>
                  ),
                },
                {
                  key: "direction",
                  label: t("sales.entries.columns.direction"),
                  value: t(DIRECTION_LABEL_KEYS[entry.direction]),
                },
                {
                  key: "entry_date",
                  label: t("sales.entries.columns.entry_date"),
                  value: date(entry.entry_date, undefined, locale),
                },
                {
                  key: "customer",
                  label: t("sales.entries.columns.customer"),
                  value: entry.customer ? (
                    <Link
                      className="underline-offset-4 hover:underline"
                      href={routes.tenant.customers.detail(
                        slug,
                        entry.customer.uuid,
                      )}
                    >
                      {entry.customer.name}
                    </Link>
                  ) : (
                    t("sales.walk_in")
                  ),
                },
                {
                  key: "reservation",
                  label: t("sales.drawer.reservation"),
                  value: entry.reservation ? (
                    <Link
                      className="underline-offset-4 hover:underline"
                      href={routes.tenant.reservations.detail(
                        slug,
                        entry.reservation.uuid,
                      )}
                    >
                      {t("sales.reservation_number", {
                        number: entry.reservation.number,
                      })}
                    </Link>
                  ) : (
                    "—"
                  ),
                },
                {
                  key: "method",
                  label: t("sales.entries.columns.method"),
                  value: entry.method
                    ? t(METHOD_LABEL_KEYS[entry.method] ?? entry.method)
                    : "—",
                },
                {
                  key: "account",
                  label: t("sales.entries.columns.account"),
                  value: entry.cash_account?.name ?? "—",
                },
                {
                  key: "branch",
                  label: t("sales.entries.columns.branch"),
                  value: entry.branch.name,
                },
                {
                  key: "description",
                  label: t("sales.entries.columns.description"),
                  value: entry.description || "—",
                },
                {
                  key: "created_by",
                  label: t("sales.drawer.created_by"),
                  value: entry.created_by
                    ? `${entry.created_by.name} ${entry.created_by.surname}`.trim()
                    : t("sales.drawer.system"),
                },
                {
                  key: "created_at",
                  label: t("sales.entries.columns.created_at"),
                  value: datetime(entry.created_at, undefined, locale),
                },
              ],
            },
          ]}
        />
      ) : null}
    </EntityDrawer>
  );
}

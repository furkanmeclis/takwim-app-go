"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  ReservationActionDialog,
  type ActionTarget,
} from "@/features/reservations/components/reservation-action-dialog";
import { useReservationColumns } from "@/features/reservations/components/reservation-columns";
import type { ReservationRowActionHandlers } from "@/features/reservations/components/reservation-row-actions";
import { RescheduleDialog } from "@/features/reservations/components/reschedule-dialog";
import { RESERVATIONS_RESOURCE } from "@/features/reservations/constants";
import {
  useReservationsFacets,
  useReservationsList,
} from "@/features/reservations/hooks/use-reservations";
import {
  READ_ANY,
  useCanWriteReservations,
} from "@/features/reservations/lib/permissions";
import type {
  ListReservationsParams,
  ReservationListItem,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

type Period = "all" | "upcoming" | "past";

/**
 * Tenant list standard: server-side pagination / sort / search (number,
 * customer, phone), faceted filters, column visibility, row actions, row
 * click → detail, live refresh. Members with only their own calendar see
 * their own visits (the API scopes the rows).
 */
export function ReservationsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const canWrite = useCanWriteReservations();

  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RESERVATIONS_RESOURCE),
  });

  const [period, setPeriod] = useState<{ value: Period; at: string }>({
    value: "all",
    at: "",
  });
  const [actionTarget, setActionTarget] = useState<ActionTarget | null>(null);
  const [rescheduleUuid, setRescheduleUuid] = useState<string | null>(null);

  const listParams = useMemo<ListReservationsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      branch: joined(column("branch")),
      staff: joined(column("staff")),
      status: joined(column("status")),
      approval_status: joined(column("approval_status")),
      payment_status: joined(column("payment_status")),
      source: joined(column("source")),
      starts_from: period.value === "upcoming" ? period.at : undefined,
      starts_to: period.value === "past" ? period.at : undefined,
    };
  }, [listState.columnFilters, listState.params, period]);

  const listQuery = useReservationsList(listParams);
  const facetsQuery = useReservationsFacets();

  const openDetail = useCallback(
    (row: ReservationListItem) =>
      router.push(routes.tenant.reservations.detail(slug, row.uuid)),
    [router, slug],
  );

  const handlers = useMemo<ReservationRowActionHandlers>(
    () => ({
      onView: openDetail,
      onAction: (row, action) =>
        setActionTarget({
          uuid: row.uuid,
          number: row.number,
          version: row.version,
          action,
        }),
      onReschedule: (row) => setRescheduleUuid(row.uuid),
    }),
    [openDetail],
  );

  const columns = useReservationColumns({
    handlers,
    facets: facetsQuery.data,
  });

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.staff) ||
    Boolean(listParams.status) ||
    Boolean(listParams.approval_status) ||
    Boolean(listParams.payment_status) ||
    Boolean(listParams.source) ||
    period.value !== "all";

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("reservations.title")}
      description={t("reservations.description")}
      permission={READ_ANY}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reservations.title") },
      ]}
      actions={
        canWrite ? (
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.reservations.create(slug))}
            label={t("reservations.actions.create")}
          />
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("reservations.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("reservations.empty_filtered_title")
            : t("reservations.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("reservations.empty_filtered_description")
            : t("reservations.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-reservations-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <Select
              value={period.value}
              onValueChange={(value) => {
                setPeriod({
                  value: value as Period,
                  // Fixed at the choice, so the query key stays stable.
                  at: new Date().toISOString(),
                });
                listState.tableState.onPaginationChange?.((prev) => ({
                  ...prev,
                  pageIndex: 0,
                }));
              }}
            >
              <SelectTrigger
                size="sm"
                className="w-36"
                aria-label={t("reservations.period.label")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t("reservations.period.all")}
                </SelectItem>
                <SelectItem value="upcoming">
                  {t("reservations.period.upcoming")}
                </SelectItem>
                <SelectItem value="past">
                  {t("reservations.period.past")}
                </SelectItem>
              </SelectContent>
            </Select>
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <ReservationActionDialog
        target={actionTarget}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
      />
      <RescheduleDialog
        uuid={rescheduleUuid}
        onOpenChange={(open) => {
          if (!open) setRescheduleUuid(null);
        }}
      />
    </EntityPage>
  );
}

"use client";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityDetail,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { useCashAmount } from "@/features/cash/lib/format";
import { useSaleLineColumns } from "@/features/sales/components/sale-line-columns";
import { useSaleLineDialogs } from "@/features/sales/components/sale-line-dialogs";
import {
  useCanReadSales,
  useReservationBalance,
} from "@/features/sales/hooks/use-sales";
import { toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function positive(value: string) {
  return (toMinor(value) ?? 0) > 0;
}

/**
 * "Ödenen / Kalan" of a visit from the ledger: what it costs (the booked
 * total while open, the posted sale lines once completed), what was paid
 * and what is left or held as credit, with its sale lines. Shown with the
 * finance module and `tenant.finance.read`; a line opens the shared drawer.
 */
export function ReservationBalanceCard({ uuid }: { uuid: string }) {
  const canRead = useCanReadSales();
  if (!canRead) return null;
  return <ReservationBalanceContent uuid={uuid} />;
}

function ReservationBalanceContent({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const money = useCashAmount();
  const query = useReservationBalance(uuid);
  const dialogs = useSaleLineDialogs();
  const columns = useSaleLineColumns({
    onOpen: dialogs.open,
    onReverse: dialogs.reverse,
    onCorrect: dialogs.correct,
    compact: true,
  });
  const view = query.data;
  const amountOf = (value: string) => money(value, view?.currency);

  return (
    <EntitySectionCard title={t("sales.balance.reservation_title")}>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("sales.balance.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {view ? (
        <div className="space-y-6">
          <EntityDetail
            sections={[
              {
                id: "balance",
                description: view.sales_posted
                  ? t("sales.balance.expected_posted")
                  : t("sales.balance.expected_booked"),
                fields: [
                  {
                    key: "expected",
                    label: t("sales.balance.expected"),
                    value: amountOf(view.expected),
                  },
                  {
                    key: "collected",
                    label: t("sales.balance.collected"),
                    value: amountOf(view.collected),
                  },
                  ...(positive(view.refunded)
                    ? [
                        {
                          key: "refunded",
                          label: t("sales.balance.refunded"),
                          value: amountOf(view.refunded),
                        },
                      ]
                    : []),
                  {
                    key: "paid",
                    label: t("sales.balance.paid"),
                    value: (
                      <span className="font-semibold">
                        {amountOf(view.paid)}
                      </span>
                    ),
                  },
                  {
                    key: "remaining",
                    label: t("sales.balance.remaining"),
                    value: (
                      <span
                        className={
                          positive(view.remaining)
                            ? "text-destructive font-semibold"
                            : "font-semibold"
                        }
                      >
                        {amountOf(view.remaining)}
                      </span>
                    ),
                  },
                  ...(positive(view.credit)
                    ? [
                        {
                          key: "credit",
                          label: t("sales.balance.credit"),
                          value: amountOf(view.credit),
                        },
                      ]
                    : []),
                ],
              },
            ]}
          />
          <div className="space-y-2">
            <h3 className="text-sm font-medium">{t("sales.balance.lines")}</h3>
            <EntityTable
              columns={columns}
              data={view.lines}
              getRowId={(row) => row.uuid}
              onRowClick={dialogs.open}
              emptyTitle={t("sales.balance.lines_empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
                globalFilter: false,
                sorting: false,
                pagination: false,
                viewMode: true,
                persistKey: "tenant-reservation-sale-lines-v1",
              }}
            />
          </div>
        </div>
      ) : null}
      {dialogs.element}
    </EntitySectionCard>
  );
}

"use client";

import { useState } from "react";

import { StatsCard } from "@/components/common/stats-card";
import { EntityPage } from "@/components/entity";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { BusinessSelect } from "@/features/customer-panel/components/business-select";
import { CustomerReservationTable } from "@/features/customer-panel/components/customer-reservation-table";
import { useCustomerPaymentSummary } from "@/features/customer-panel/hooks/use-customer-panel";
import { amount, toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/panel/payments`: what the person owes, paid and got back, per currency,
 * from the businesses' payment ledgers (TEC-967), and every visit with
 * money on it. Visits cancelled before anything was paid are left out by
 * the API.
 */
export function CustomerPaymentsPage() {
  const { t, locale } = useLocale();
  const [business, setBusiness] = useState<string | undefined>();
  const summary = useCustomerPaymentSummary();
  const totals = summary.data?.totals ?? [];

  return (
    <EntityPage
      title={t("customer_panel.payments.title")}
      description={t("customer_panel.payments.description")}
    >
      {summary.isLoading ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Skeleton className="h-28" />
          <Skeleton className="h-28" />
          <Skeleton className="h-28" />
          <Skeleton className="h-28" />
        </div>
      ) : null}
      {totals.map((total) => (
        <div
          key={total.currency}
          className="grid grid-cols-2 gap-3 lg:grid-cols-4"
        >
          <StatsCard
            title={t("customer_panel.payments.booked")}
            value={amount(total.booked, total.currency, locale)}
            hint={t("customer_panel.payments.visits", {
              count: total.reservation_count,
            })}
          />
          <StatsCard
            title={t("customer_panel.payments.paid")}
            value={amount(total.paid, total.currency, locale)}
          />
          <StatsCard
            title={t("customer_panel.payments.refunded")}
            value={amount(total.refunded, total.currency, locale)}
            hint={
              toMinor(total.refund_pending)
                ? t("customer_panel.payments.refund_pending", {
                    amount: amount(
                      total.refund_pending,
                      total.currency,
                      locale,
                    ),
                  })
                : undefined
            }
          />
          <StatsCard
            title={t("customer_panel.payments.balance")}
            value={amount(total.balance, total.currency, locale)}
          />
        </div>
      ))}
      <Alert>
        <AlertDescription>{t("customer_panel.payments.note")}</AlertDescription>
      </Alert>
      <CustomerReservationTable
        scope="all"
        money
        business={business}
        toolbarExtra={
          <BusinessSelect value={business} onChange={setBusiness} />
        }
        persistKey="customer-panel-payments-v1"
        emptyTitle={t("customer_panel.payments.empty")}
        emptyDescription={t("customer_panel.payments.empty_description")}
      />
    </EntityPage>
  );
}

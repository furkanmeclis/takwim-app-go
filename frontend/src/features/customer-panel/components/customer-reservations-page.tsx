"use client";

import { useState } from "react";

import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BusinessSelect } from "@/features/customer-panel/components/business-select";
import { CustomerReservationTable } from "@/features/customer-panel/components/customer-reservation-table";
import { useLocale } from "@/providers/locale-provider";

type Tab = "upcoming" | "past";

/**
 * `/panel`: the person's visits at every business that knows their phone,
 * upcoming first (soonest on top), and the history (newest on top).
 */
export function CustomerReservationsPage() {
  const { t } = useLocale();
  const [tab, setTab] = useState<Tab>("upcoming");
  const [business, setBusiness] = useState<string | undefined>();
  const filter = <BusinessSelect value={business} onChange={setBusiness} />;

  return (
    <EntityPage
      title={t("customer_panel.reservations.title")}
      description={t("customer_panel.reservations.description")}
    >
      <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)}>
        <TabsList className="w-full sm:w-auto">
          <TabsTrigger value="upcoming" className="flex-1 sm:flex-none">
            {t("customer_panel.reservations.upcoming")}
          </TabsTrigger>
          <TabsTrigger value="past" className="flex-1 sm:flex-none">
            {t("customer_panel.reservations.past")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="upcoming" className="mt-4">
          <CustomerReservationTable
            scope="upcoming"
            business={business}
            toolbarExtra={filter}
            persistKey="customer-panel-upcoming-v1"
            emptyTitle={t("customer_panel.reservations.empty_upcoming")}
            emptyDescription={t(
              "customer_panel.reservations.empty_upcoming_description",
            )}
          />
        </TabsContent>
        <TabsContent value="past" className="mt-4">
          <CustomerReservationTable
            scope="past"
            business={business}
            toolbarExtra={filter}
            persistKey="customer-panel-past-v1"
            emptyTitle={t("customer_panel.reservations.empty_past")}
            emptyDescription={t(
              "customer_panel.reservations.empty_past_description",
            )}
          />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}

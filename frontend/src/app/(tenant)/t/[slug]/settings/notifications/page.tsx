import { Suspense } from "react";

import { CustomerNotificationsPage } from "@/features/customer-notifications";

export default function TenantCustomerNotificationsRoute() {
  return (
    <Suspense>
      <CustomerNotificationsPage />
    </Suspense>
  );
}

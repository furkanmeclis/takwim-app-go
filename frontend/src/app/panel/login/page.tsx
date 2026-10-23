import { Suspense } from "react";

import { Loading } from "@/components/common/loading";
import { CustomerLoginPage } from "@/features/customer-panel/components/customer-login-page";

export default function CustomerLoginRoute() {
  return (
    <Suspense fallback={<Loading />}>
      <CustomerLoginPage />
    </Suspense>
  );
}

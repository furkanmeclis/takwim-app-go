import { Suspense } from "react";

import { LogsPage } from "@/features/logs";

export default function Page() {
  return (
    <Suspense>
      <LogsPage />
    </Suspense>
  );
}

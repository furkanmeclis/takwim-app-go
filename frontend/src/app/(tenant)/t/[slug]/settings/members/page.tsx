import { Suspense } from "react";

import { MembersPage } from "@/features/tenant-members";

export default function TenantMembersRoute() {
  return (
    <Suspense>
      <MembersPage />
    </Suspense>
  );
}

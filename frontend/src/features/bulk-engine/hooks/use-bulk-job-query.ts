import { useQuery } from "@tanstack/react-query";

import { bulkService } from "@/features/bulk-engine/services/bulk.service";
import { useApiScope } from "@/features/tenant/hooks/use-api-scope";
import { scopeKey } from "@/lib/api/scoped-request";

const ACTIVE_STATUSES = new Set(["queued", "processing"]);

export function useBulkJobQuery(uuid: string | null, enabled = true) {
  const scope = useApiScope();
  return useQuery({
    queryKey: ["bulk-jobs", scopeKey(scope), uuid],
    queryFn: () => bulkService.get(uuid!, scope),
    enabled: Boolean(uuid) && enabled,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status && ACTIVE_STATUSES.has(status)) return 2000;
      return false;
    },
  });
}

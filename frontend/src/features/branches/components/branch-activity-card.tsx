"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useBranchActivity } from "@/features/branches/hooks/use-branches";
import type { BranchActivityEntry } from "@/features/branches/types";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

function actorName(entry: BranchActivityEntry) {
  if (!entry.actor) return null;
  const full = `${entry.actor.name} ${entry.actor.surname}`.trim();
  return full || entry.actor.email;
}

/** Audit trail of a branch (`GET /v1/tenant/branches/{uuid}/activity`). */
export function BranchActivityCard({ branchUuid }: { branchUuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(PAGE_SIZE);
  const query = useBranchActivity(branchUuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const describe = (entry: BranchActivityEntry) => {
    const actionKey = `activity.actions.${entry.action}`;
    const action = t(actionKey);
    const payload = entry.payload as Record<string, unknown>;
    const pointName =
      typeof payload.name === "string" && entry.action.includes("service_point")
        ? ` · ${payload.name}`
        : "";
    return (action === actionKey ? entry.action : action) + pointName;
  };

  return (
    <EntitySectionCard
      title={t("branches.detail.activity")}
      badge={query.data ? total : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <EmptyState className="py-8" title={t("branches.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry),
              description:
                actorName(entry) ?? t("branches.activity.system_actor"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetching}
              onClick={() => setLimit((current) => current + PAGE_SIZE)}
            >
              {t("branches.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

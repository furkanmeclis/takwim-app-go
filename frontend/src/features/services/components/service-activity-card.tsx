"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useServiceActivity } from "@/features/services/hooks/use-services";
import type { ServiceActivityPage } from "@/features/services/types";
import { activityActionLabel } from "@/features/tenant-activity/lib/labels";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

type ActivityEntry = ServiceActivityPage["items"][number];

function actorName(entry: ActivityEntry) {
  if (!entry.actor) return null;
  const full = `${entry.actor.name} ${entry.actor.surname}`.trim();
  return full || entry.actor.email;
}

/** Audit trail of a service (`GET /v1/tenant/services/{uuid}/activity`). */
export function ServiceActivityCard({ serviceUuid }: { serviceUuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(PAGE_SIZE);
  const query = useServiceActivity(serviceUuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const describe = (entry: ActivityEntry) => {
    return activityActionLabel(t, entry.action);
  };

  return (
    <EntitySectionCard
      title={t("services.detail.activity")}
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
        <EmptyState className="py-8" title={t("services.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry),
              description:
                actorName(entry) ?? t("services.activity.system_actor"),
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
              {t("services.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

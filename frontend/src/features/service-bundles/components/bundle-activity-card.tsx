"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useServiceBundleActivity } from "@/features/service-bundles/hooks/use-service-bundles";
import type { ServiceBundleActivityPage } from "@/features/service-bundles/types";
import { activityActionLabel } from "@/features/tenant-activity/lib/labels";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

type ActivityEntry = ServiceBundleActivityPage["items"][number];

function actorName(entry: ActivityEntry) {
  if (!entry.actor) return null;
  const full = `${entry.actor.name} ${entry.actor.surname}`.trim();
  return full || entry.actor.email;
}

/** Audit trail of a bundle (`GET /v1/tenant/service-bundles/{uuid}/activity`). */
export function BundleActivityCard({ bundleUuid }: { bundleUuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(PAGE_SIZE);
  const query = useServiceBundleActivity(bundleUuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  return (
    <EntitySectionCard
      title={t("service_bundles.detail.activity")}
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
        <EmptyState
          className="py-8"
          title={t("service_bundles.activity.empty")}
        />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: activityActionLabel(t, entry.action),
              description:
                actorName(entry) ?? t("service_bundles.activity.system_actor"),
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
              {t("service_bundles.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

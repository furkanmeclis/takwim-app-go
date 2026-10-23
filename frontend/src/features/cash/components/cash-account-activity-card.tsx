"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useCashAccountActivity } from "@/features/cash/hooks/use-cash";
import { useCashAmount } from "@/features/cash/lib/format";
import type { CashActivityEntry } from "@/features/cash/types";
import { activityActionLabel } from "@/features/tenant-activity/lib/labels";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

function actorName(entry: CashActivityEntry) {
  if (!entry.actor) return null;
  const full = `${entry.actor.name} ${entry.actor.surname}`.trim();
  return full || entry.actor.email;
}

/** Audit trail of an account: edits, movements, transfers, closings (`GET /v1/tenant/cash/accounts/{uuid}/activity`). */
export function CashAccountActivityCard({
  accountUuid,
}: {
  accountUuid: string;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  const [limit, setLimit] = useState(PAGE_SIZE);
  const query = useCashAccountActivity(accountUuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const describe = (entry: CashActivityEntry) => {
    const payload = entry.payload as Record<string, unknown>;
    const parts = [activityActionLabel(t, entry.action)];
    if (typeof payload.amount === "string" && payload.amount) {
      parts.push(
        money(
          payload.amount,
          typeof payload.currency === "string" ? payload.currency : undefined,
        ),
      );
    }
    for (const key of ["counter_account", "description", "reason"]) {
      const value = payload[key];
      if (typeof value === "string" && value) parts.push(value);
    }
    return parts.join(" · ");
  };

  return (
    <EntitySectionCard
      title={t("cash.detail.activity")}
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
        <EmptyState className="py-8" title={t("cash.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry),
              description: actorName(entry) ?? t("cash.activity.system_actor"),
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
              {t("cash.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

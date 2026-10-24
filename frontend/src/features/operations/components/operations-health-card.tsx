"use client";

import Link from "next/link";
import { ListChecks } from "lucide-react";
import type { ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useOperationsSummary } from "@/features/operations/hooks/use-operations";
import { useLocale } from "@/providers/locale-provider";

function Line({
  label,
  children,
  last = false,
}: {
  label: string;
  children: ReactNode;
  last?: boolean;
}) {
  return (
    <div
      className={
        last
          ? "flex items-center justify-between gap-3 py-2"
          : "border-border/50 flex items-center justify-between gap-3 border-b py-2"
      }
    >
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono font-medium">{children}</span>
    </div>
  );
}

function Count({ value, tone }: { value: number; tone: "warning" | "danger" }) {
  return value > 0 ? (
    <StatusChip label={String(value)} tone={tone} />
  ) : (
    <span>0</span>
  );
}

/** Background work on the system health page (TEC-930), with a link to the failure lists. */
export function OperationsHealthCard() {
  const { t } = useLocale();
  const query = useOperationsSummary();
  const s = query.data;
  return (
    <Card className="border-border/60 shadow-xs">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold">
          <ListChecks className="text-primary size-4" />
          {t("operations.health.title")}
        </CardTitle>
        {s ? (
          <StatusChip
            label={t(
              s.status === "healthy"
                ? "operations.summary.status_healthy"
                : "operations.summary.status_attention",
            )}
            tone={s.status === "healthy" ? "success" : "warning"}
          />
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3 text-xs">
        {query.isError ? (
          <p className="text-muted-foreground">
            {t("operations.summary.load_failed")}
          </p>
        ) : !s ? (
          <p className="text-muted-foreground">{t("common.loading")}</p>
        ) : (
          <div>
            <Line label={t("operations.summary.workers")}>
              {s.queue.available ? (
                s.queue.workers > 0 ? (
                  s.queue.workers
                ) : (
                  <StatusChip
                    label={t("operations.summary.workers_none")}
                    tone="danger"
                  />
                )
              ) : (
                <StatusChip
                  label={t("operations.summary.queue_unavailable")}
                  tone="danger"
                />
              )}
            </Line>
            <Line label={t("operations.summary.failed_tasks")}>
              <Count value={s.queue.archived} tone="danger" />
            </Line>
            <Line label={t("operations.summary.provider_errors")}>
              <Count
                value={
                  s.integrations.failed +
                  s.integrations.needs_verification +
                  s.integrations.blocked
                }
                tone="warning"
              />
            </Line>
            <Line label={t("operations.summary.notice_failures")}>
              <Count value={s.notices.failed_24h} tone="warning" />
            </Line>
            <Line label={t("operations.summary.error_tracking")} last>
              <StatusChip
                label={t(
                  s.error_tracking.enabled
                    ? "operations.summary.tracking_api_on"
                    : "operations.summary.tracking_api_off",
                )}
                tone={s.error_tracking.enabled ? "success" : "warning"}
              />
            </Line>
          </div>
        )}
        <Button
          asChild
          variant="outline"
          size="sm"
          className="w-full sm:w-auto"
        >
          <Link href={routes.platform.settings.operations}>
            {t("operations.health.open")}
          </Link>
        </Button>
      </CardContent>
    </Card>
  );
}

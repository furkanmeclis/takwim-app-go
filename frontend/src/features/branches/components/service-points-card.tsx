"use client";

import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ServicePointDialog } from "@/features/branches/components/service-point-dialog";
import { SERVICE_POINT_TYPE_LABEL_KEYS } from "@/features/branches/constants";
import {
  useDeleteServicePoint,
  useServicePoints,
} from "@/features/branches/hooks/use-branches";
import type { ServicePoint, WeeklyHours } from "@/features/branches/types";
import { money } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Service points sub-list of a branch, with add / edit / delete dialogs. */
export function ServicePointsCard({
  branchUuid,
  branchHours,
}: {
  branchUuid: string;
  branchHours: WeeklyHours;
}) {
  const { t, locale } = useLocale();
  const query = useServicePoints(branchUuid);
  const remove = useDeleteServicePoint(branchUuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    point: ServicePoint | null;
  }>({ open: false, point: null });
  const [deleteTarget, setDeleteTarget] = useState<ServicePoint | null>(null);
  const points = query.data?.items ?? [];

  const addButton = (
    <PermissionGuard permission={permissions.tenant.branches.write}>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => setDialog({ open: true, point: null })}
      >
        <Plus className="size-4" />
        {t("branches.service_points.add")}
      </Button>
    </PermissionGuard>
  );

  return (
    <EntitySectionCard
      title={t("branches.detail.service_points")}
      badge={query.data ? points.length : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && points.length === 0 ? (
        <EmptyState
          className="py-10"
          title={t("branches.service_points.empty_title")}
          description={t("branches.service_points.empty_description")}
          action={addButton}
        />
      ) : null}
      {points.length > 0 ? (
        <div className="space-y-3">
          <div className="flex justify-end">{addButton}</div>
          <ul className="divide-border border-border divide-y rounded-lg border">
            {points.map((point) => (
              <li
                key={point.uuid}
                className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{point.name}</span>
                    <Badge variant="secondary">
                      {t(SERVICE_POINT_TYPE_LABEL_KEYS[point.type])}
                    </Badge>
                    {!point.is_active ? (
                      <StatusChip label={t("branches.status.inactive")} />
                    ) : null}
                    {point.online_selectable ? (
                      <Badge variant="outline">
                        {t("branches.service_points.online")}
                      </Badge>
                    ) : null}
                  </div>
                  <p className="text-muted-foreground text-xs">
                    {point.description || "—"} ·{" "}
                    {t("branches.service_points.fee", {
                      amount: money(Number(point.extra_fee), locale),
                    })}{" "}
                    ·{" "}
                    {point.working_hours
                      ? t("branches.service_points.own_hours")
                      : t("branches.service_points.branch_hours")}
                  </p>
                </div>
                <EntityRowActions
                  actions={[
                    {
                      id: "edit",
                      label: t("branches.actions.edit"),
                      icon: Pencil,
                      permission: permissions.tenant.branches.write,
                      onSelect: () => setDialog({ open: true, point }),
                    },
                    {
                      id: "delete",
                      label: t("branches.actions.delete"),
                      icon: Trash2,
                      variant: "destructive",
                      permission: permissions.tenant.branches.write,
                      onSelect: () => setDeleteTarget(point),
                    },
                  ]}
                />
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <ServicePointDialog
        branchUuid={branchUuid}
        branchHours={branchHours}
        open={dialog.open}
        point={dialog.point}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, point: open ? prev.point : null }))
        }
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import type { ReactNode } from "react";

import {
  EntityDrawer,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  activityActionLabel,
  activityActorName,
  activityChanges,
  activityDetails,
  activityFieldLabel,
  activityResourceLabel,
  formatActivityValue,
} from "@/features/tenant-activity/lib/labels";
import type { TenantActivityEvent } from "@/features/tenant-activity/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type ChangeRow = { path: string; before: unknown; after: unknown };
type DetailRow = { key: string; value: unknown };

const STATIC_TABLE = {
  manual: { filtering: false, sorting: false, pagination: false },
  features: {
    rowSelection: false,
    columnFilters: false,
    facetedFilters: false,
    sorting: false,
    pagination: false,
  },
} as const;

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 py-1.5 text-sm">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="min-w-0 text-right font-medium break-words">
        {value}
      </span>
    </div>
  );
}

/**
 * One history entry: who did what, where and when, then the before / after
 * of every changed field and the remaining action details.
 */
export function ActivityEventDrawer({
  event,
  onOpenChange,
}: {
  event: TenantActivityEvent | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const changes = event ? activityChanges(event) : [];
  const details = event ? activityDetails(event) : [];
  const resource = event?.resource ?? "";

  const changeColumns = [
    createColumn<ChangeRow>({
      accessorKey: "path",
      labelKey: "activity.history.field",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">
          {activityFieldLabel(t, resource, row.original.path)}
        </span>
      ),
    }),
    createColumn<ChangeRow>({
      accessorKey: "before",
      labelKey: "activity.history.before",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground break-words whitespace-pre-wrap line-through decoration-1">
          {formatActivityValue(t, row.original.before)}
        </span>
      ),
    }),
    createColumn<ChangeRow>({
      accessorKey: "after",
      labelKey: "activity.history.after",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <span className="break-words whitespace-pre-wrap">
          {formatActivityValue(t, row.original.after)}
        </span>
      ),
    }),
  ] as ColumnDef<ChangeRow, unknown>[];

  const detailColumns = [
    createColumn<DetailRow>({
      accessorKey: "key",
      labelKey: "activity.history.field",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">
          {activityFieldLabel(t, resource, row.original.key)}
        </span>
      ),
    }),
    createColumn<DetailRow>({
      accessorKey: "value",
      labelKey: "activity.history.value",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <span className="break-words whitespace-pre-wrap">
          {formatActivityValue(t, row.original.value)}
        </span>
      ),
    }),
  ] as ColumnDef<DetailRow, unknown>[];

  return (
    <EntityDrawer
      open={Boolean(event)}
      onOpenChange={onOpenChange}
      title={event ? activityActionLabel(t, event.action) : ""}
      description={
        event ? datetime(event.created_at, undefined, locale) : undefined
      }
      size="lg"
    >
      {event ? (
        <div className="space-y-4">
          <div className="divide-y">
            <Row
              label={t("activity.columns.resource")}
              value={activityResourceLabel(t, event.resource)}
            />
            <Row
              label={t("activity.history.branch")}
              value={event.branch?.name ?? t("activity.history.whole_business")}
            />
            <Row
              label={t("activity.columns.actor")}
              value={activityActorName(event) ?? t("activity.history.system")}
            />
            {event.actor?.email ? (
              <Row
                label={t("activity.history.email")}
                value={event.actor.email}
              />
            ) : null}
          </div>
          <EntitySectionCard
            title={t("activity.history.changes")}
            badge={changes.length || undefined}
          >
            <EntityTable
              columns={changeColumns}
              data={changes}
              getRowId={(row) => row.path}
              emptyTitle={t("activity.history.no_changes")}
              {...STATIC_TABLE}
              features={{
                ...STATIC_TABLE.features,
                persistKey: "tenant-activity-changes-v1",
              }}
            />
          </EntitySectionCard>
          {details.length > 0 ? (
            <EntitySectionCard title={t("activity.history.details")}>
              <EntityTable
                columns={detailColumns}
                data={details}
                getRowId={(row) => row.key}
                emptyTitle={t("activity.history.no_details")}
                {...STATIC_TABLE}
                features={{
                  ...STATIC_TABLE.features,
                  persistKey: "tenant-activity-details-v1",
                }}
              />
            </EntitySectionCard>
          ) : null}
        </div>
      ) : null}
    </EntityDrawer>
  );
}

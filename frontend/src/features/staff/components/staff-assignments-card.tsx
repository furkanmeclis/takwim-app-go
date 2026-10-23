"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CalendarClock } from "lucide-react";
import { useState } from "react";

import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { DAY_LABEL_KEYS } from "@/features/branches/constants";
import { ShiftEditorDialog } from "@/features/staff/components/shift-editor-dialog";
import { SHIFT_DAY_KEYS } from "@/features/staff/constants";
import { hoursLabel, weekMinutes } from "@/features/staff/lib/shifts";
import type { StaffAssignment } from "@/features/staff/types";
import { useLocale } from "@/providers/locale-provider";

const STATIC_TABLE = {
  manual: { filtering: false, sorting: false, pagination: false },
  features: {
    rowSelection: false,
    columnFilters: false,
    facetedFilters: false,
    globalFilter: false,
    sorting: false,
    pagination: false,
  },
} as const;

/** Branches of the person with the weekly shifts in each. */
export function StaffAssignmentsCard({
  staffUuid,
  assignments,
}: {
  staffUuid: string;
  assignments: StaffAssignment[];
}) {
  const { t, locale } = useLocale();
  const [editing, setEditing] = useState<StaffAssignment | null>(null);

  const columns = [
    createColumn<StaffAssignment>({
      accessorKey: "branch_name",
      labelKey: "staff.assignments.branch",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <span className="font-medium">{row.original.branch_name}</span>
          <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
            {row.original.branch_code}
          </code>
          {!row.original.editable ? (
            <Badge variant="outline">{t("staff.assignments.read_only")}</Badge>
          ) : null}
        </div>
      ),
    }),
    createColumn<StaffAssignment>({
      id: "week",
      labelKey: "staff.assignments.week",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => {
        const days = SHIFT_DAY_KEYS.filter(
          (day) => row.original.shifts[day].length > 0,
        );
        if (!days.length) {
          return (
            <span className="text-muted-foreground text-sm">
              {t("staff.assignments.no_shifts")}
            </span>
          );
        }
        return (
          <div className="flex flex-wrap gap-1">
            {days.map((day) => (
              <Badge key={day} variant="secondary" className="tabular-nums">
                {t(DAY_LABEL_KEYS[day])}{" "}
                {row.original.shifts[day]
                  .map((shift) => `${shift.start}–${shift.end}`)
                  .join(", ")}
              </Badge>
            ))}
          </div>
        );
      },
    }),
    createColumn<StaffAssignment>({
      id: "hours",
      labelKey: "staff.assignments.hours",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="tabular-nums">
          {t("staff.assignments.hours_value", {
            hours: hoursLabel(weekMinutes(row.original.shifts), locale),
          })}
        </span>
      ),
    }),
    createColumn<StaffAssignment>({
      id: "actions",
      labelKey: "staff.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) =>
        row.original.editable ? (
          <EntityRowActions
            actions={[
              {
                id: "shifts",
                label: t("staff.shifts.edit"),
                icon: CalendarClock,
                permission: permissions.tenant.schedules.write,
                onSelect: () => setEditing(row.original),
              },
            ]}
          />
        ) : null,
    }),
  ] as ColumnDef<StaffAssignment, unknown>[];

  return (
    <EntitySectionCard
      title={t("staff.detail.assignments")}
      badge={assignments.length}
    >
      <EntityTable
        columns={columns}
        data={assignments}
        getRowId={(row) => row.branch_uuid}
        onRowClick={(row) => {
          if (row.editable) setEditing(row);
        }}
        emptyTitle={t("staff.assignments.empty_title")}
        emptyDescription={t("staff.assignments.empty_description")}
        {...STATIC_TABLE}
        features={{
          ...STATIC_TABLE.features,
          persistKey: "tenant-staff-assignments-v1",
        }}
      />
      <ShiftEditorDialog
        staffUuid={staffUuid}
        assignment={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />
    </EntitySectionCard>
  );
}

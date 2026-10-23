"use client";

import type { ColumnDef, RowData } from "@tanstack/react-table";

import { Checkbox } from "@/components/ui/checkbox";

export function createSelectColumnDef<TData extends RowData>(): ColumnDef<
  TData,
  unknown
> {
  return {
    id: "__select",
    size: 40,
    enableSorting: false,
    enableHiding: false,
    enableColumnFilter: false,
    enableResizing: false,
    enablePinning: true,
    meta: {
      labelKey: "table.select",
      enableHiding: false,
    },
    header: ({ table }) => (
      <Checkbox
        checked={
          table.getIsAllPageRowsSelected() ||
          (table.getIsSomePageRowsSelected() && "indeterminate")
        }
        onCheckedChange={(value) =>
          table.toggleAllPageRowsSelected(Boolean(value))
        }
        aria-label="Select all"
      />
    ),
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        disabled={!row.getCanSelect()}
        onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
        aria-label="Select row"
      />
    ),
  };
}

"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { EmptyState } from "@/components/common/empty-state";
import { DataTablePagination } from "@/components/tables/data-table-pagination";
import { DataTableToolbar } from "@/components/tables/data-table-toolbar";
import { GridView } from "@/components/tables/grid-view";
import { LiveUpdatesBanner } from "@/components/tables/live-updates-banner";
import { createReorderColumnDef } from "@/components/tables/reorder-column";
import { reorderRowsById } from "@/components/tables/reorder-rows";
import { TableView } from "@/components/tables/table-view";
import type { DataTableProps } from "@/components/tables/types";
import { useDataTable } from "@/components/tables/use-data-table";
import { useLiveRows } from "@/components/tables/use-live-rows";
import { Skeleton } from "@/components/ui/skeleton";
import { useIsMobile } from "@/hooks/use-mobile";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

/** Distance (px) from the top after which the user counts as "scrolled". */
const SCROLLED_PX = 48;

function scrollParent(node: HTMLElement | null): HTMLElement | null {
  let el = node?.parentElement ?? null;
  while (el) {
    const { overflowY } = window.getComputedStyle(el);
    if (
      (overflowY === "auto" || overflowY === "scroll") &&
      el.scrollHeight > el.clientHeight
    ) {
      return el;
    }
    el = el.parentElement;
  }
  return null;
}

export function DataTable<TData>({
  columns,
  data,
  getRowId,
  isLoading,
  emptyTitle,
  emptyDescription,
  features,
  manual,
  state,
  initialState,
  pageCount,
  pageSizeOptions,
  toolbar,
  bulkActions,
  renderGridItem,
  onRowReorder,
  onCellEdit,
  onExportReady,
  onRowClick,
  realtime,
  className,
}: DataTableProps<TData>) {
  const { t } = useLocale();
  const isMobile = useIsMobile();
  const [reorderModeRequested, setReorderModeRequested] = useState(false);
  const [editingCell, setEditingCell] = useState<{
    rowId: string;
    columnId: string;
  } | null>(null);

  const resolvedGetRowId =
    getRowId ??
    ((row: TData, index: number) => {
      const maybe = row as { id?: string | number };
      return maybe.id != null ? String(maybe.id) : String(index);
    });

  const forceMobileCardsPreview =
    isMobile && features?.mobileAutoCards !== false;
  const canReorder =
    Boolean(features?.rowReorder) &&
    Boolean(onRowReorder) &&
    !forceMobileCardsPreview;
  const reorderMode = canReorder && reorderModeRequested;

  const columnsWithReorder = useMemo(() => {
    if (!reorderMode) return columns;
    return [createReorderColumnDef<TData>(), ...columns];
  }, [columns, reorderMode]);

  const {
    table,
    features: resolvedFeatures,
    density,
    setDensity,
    viewMode,
    setViewMode,
    resetAll,
  } = useDataTable({
    columns: columnsWithReorder,
    data,
    getRowId: resolvedGetRowId,
    features,
    manual,
    state,
    initialState,
    pageCount,
  });

  const forceMobileCards =
    isMobile && resolvedFeatures.mobileAutoCards !== false;

  const displayMode = reorderMode
    ? "table"
    : forceMobileCards
      ? "grid"
      : viewMode;

  const activeEditingCell = reorderMode ? null : editingCell;

  // Live updates (opt-in): see use-live-rows for the refresh rules.
  const rootRef = useRef<HTMLDivElement>(null);
  const pageIndex = table.getState().pagination.pageIndex;
  const isScrolled = useCallback(() => {
    if (typeof window === "undefined") return false;
    const parent = scrollParent(rootRef.current);
    const top = parent ? parent.scrollTop : window.scrollY;
    return top > SCROLLED_PX;
  }, []);
  const live = useLiveRows({
    realtime,
    rowIds: data.map((row, index) => resolvedGetRowId(row, index)),
    pageIndex,
    isScrolled,
    paused: reorderMode || activeEditingCell !== null,
    onRowsDeleted: (ids) => {
      const selection = table.getState().rowSelection;
      if (!ids.some((id) => selection[id])) return;
      table.setRowSelection((prev) => {
        const next = { ...prev };
        for (const id of ids) delete next[id];
        return next;
      });
    },
  });
  // "Refresh" from page N jumps to page 1; refetch once that query is current.
  const refreshOnFirstPage = useRef(false);
  const refetchLive = realtime?.refetch;
  useEffect(() => {
    if (!refreshOnFirstPage.current || pageIndex !== 0) return;
    refreshOnFirstPage.current = false;
    void refetchLive?.();
  }, [pageIndex, refetchLive]);
  const applyLiveUpdates = () => {
    if (pageIndex !== 0) {
      refreshOnFirstPage.current = true;
      live.dismiss();
      table.setPageIndex(0);
    } else {
      live.applyPending();
    }
    if (isScrolled()) {
      rootRef.current?.scrollIntoView({ block: "start", behavior: "smooth" });
    }
  };

  const toolbarNode: ReactNode =
    typeof toolbar === "function" ? toolbar(table) : toolbar;

  if (isLoading) {
    return (
      <div className={cn("space-y-3", className)}>
        <Skeleton className="h-9 w-full max-w-sm" />
        {forceMobileCards || displayMode === "grid" ? (
          <div className="grid grid-cols-1 gap-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-36 w-full rounded-xl" />
            ))}
          </div>
        ) : (
          <div className="space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        )}
      </div>
    );
  }

  const filteredCount = table.getFilteredRowModel().rows.length;
  const hasRows = table.getRowModel().rows.length > 0;
  const columnCount = table.getVisibleLeafColumns().length;

  const emptyMessage = !data.length
    ? {
        title: emptyTitle ?? t("common.empty_title"),
        description: emptyDescription ?? t("common.empty_description"),
      }
    : !filteredCount
      ? {
          title: t("table.no_results"),
          description: t("table.reset_filters"),
        }
      : null;

  return (
    <div ref={rootRef} className={cn("space-y-3", className)}>
      <DataTableToolbar
        table={table}
        features={resolvedFeatures}
        density={density}
        setDensity={setDensity}
        viewMode={viewMode}
        setViewMode={setViewMode}
        resetAll={resetAll}
        toolbar={toolbarNode}
        bulkActions={bulkActions}
        forceMobileCards={forceMobileCards}
        reorderMode={reorderMode}
        onReorderModeChange={
          canReorder
            ? (next) => {
                if (next) {
                  setViewMode("table");
                  table.resetSorting();
                  setEditingCell(null);
                }
                setReorderModeRequested(next);
              }
            : undefined
        }
        onExport={
          onExportReady
            ? () =>
                onExportReady(
                  table.getFilteredRowModel().rows.map((r) => r.original),
                  table,
                )
            : undefined
        }
      />

      {reorderMode ? (
        <p className="text-muted-foreground text-xs">
          {t("table.reorder_hint")}
        </p>
      ) : null}

      {live.hasPending ? (
        <LiveUpdatesBanner
          pending={live.pending}
          onRefresh={applyLiveUpdates}
          onDismiss={live.dismiss}
        />
      ) : null}

      {displayMode === "grid" ? (
        emptyMessage && !hasRows ? (
          <div className="space-y-3">
            <EmptyState
              title={emptyMessage.title}
              description={emptyMessage.description}
            />
          </div>
        ) : (
          <GridView
            table={table}
            renderGridItem={renderGridItem}
            mobile={forceMobileCards}
            onRowClick={reorderMode ? undefined : onRowClick}
            fadingRowIds={live.fadingIds}
          />
        )
      ) : (
        <TableView
          table={table}
          density={density}
          showColumnFilters={
            resolvedFeatures.columnFilters && !isMobile && !reorderMode
          }
          emptyMessage={emptyMessage}
          columnCount={columnCount}
          reorderMode={reorderMode}
          onReorder={(activeId, overId) => {
            if (!onRowReorder) return;
            onRowReorder(
              reorderRowsById(data, resolvedGetRowId, activeId, overId),
            );
          }}
          inlineEdit={
            Boolean(resolvedFeatures.inlineEdit) &&
            Boolean(onCellEdit) &&
            !reorderMode
          }
          editingCell={activeEditingCell}
          onEditingCellChange={setEditingCell}
          onCellEdit={onCellEdit}
          onRowClick={reorderMode ? undefined : onRowClick}
          fadingRowIds={live.fadingIds}
        />
      )}

      {resolvedFeatures.pagination && hasRows && !reorderMode ? (
        <DataTablePagination
          table={table}
          pageSizeOptions={pageSizeOptions}
          compact={forceMobileCards}
        />
      ) : null}
    </div>
  );
}

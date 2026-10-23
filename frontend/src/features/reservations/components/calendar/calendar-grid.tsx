"use client";

import { BellRing } from "lucide-react";
import { useMemo, useRef, type DragEvent, type MouseEvent } from "react";

import { canReschedule } from "@/features/reservations/constants";
import {
  clock,
  dateRange,
  zoned,
} from "@/features/reservations/lib/calendar-time";
import { statusLabel } from "@/features/reservations/lib/labels";
import type {
  CalendarColumn,
  CalendarInterval,
  CalendarItem,
  ReservationCalendar,
} from "@/features/reservations/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Pixels of one 15 minute row. */
const SLOT = 14;
const STEP = 15;

export type CalendarView = "day" | "week";

/** A place on the grid: a local date, a minute of the day, a column. */
export type GridPoint = {
  date: string;
  minutes: number;
  column: CalendarColumn | null;
};

type CalendarGridProps = {
  calendar: ReservationCalendar;
  view: CalendarView;
  /** Day view: the date shown. */
  date: string;
  /** Week view: one column's work and visits, or every visit ("all"). */
  columnKey: string;
  canWrite: boolean;
  onItemClick: (item: CalendarItem) => void;
  onEmptyClick?: (point: GridPoint) => void;
  onMove?: (item: CalendarItem, point: GridPoint) => void;
};

type Lane = { item: CalendarItem; lane: number; lanes: number };

/** Side by side lanes for visits that overlap in one cell. */
function layout(items: CalendarItem[]): Lane[] {
  const sorted = [...items].sort(
    (a, b) =>
      a.starts_at.localeCompare(b.starts_at) ||
      a.ends_at.localeCompare(b.ends_at),
  );
  const out: Lane[] = [];
  let cluster: Lane[] = [];
  let clusterEnd = "";
  let ends: string[] = [];
  const close = () => {
    for (const entry of cluster) entry.lanes = ends.length;
    out.push(...cluster);
    cluster = [];
    ends = [];
    clusterEnd = "";
  };
  for (const item of sorted) {
    if (cluster.length && item.starts_at >= clusterEnd) close();
    let lane = ends.findIndex((end) => end <= item.starts_at);
    if (lane === -1) {
      lane = ends.length;
      ends.push(item.ends_at);
    } else {
      ends[lane] = item.ends_at;
    }
    cluster.push({ item, lane, lanes: 1 });
    if (item.ends_at > clusterEnd) clusterEnd = item.ends_at;
  }
  if (cluster.length) close();
  return out;
}

const ITEM_TONE: Record<string, string> = {
  requested:
    "border-dashed border-amber-500 bg-amber-50 text-amber-950 dark:bg-amber-950/50 dark:text-amber-50",
  active: "border-primary bg-primary/10 text-foreground",
  in_service:
    "border-emerald-600 bg-emerald-50 text-emerald-950 dark:bg-emerald-950/50 dark:text-emerald-50",
  completed: "border-border bg-muted text-muted-foreground",
  cancelled:
    "border-destructive/60 bg-destructive/10 text-muted-foreground line-through",
  rejected:
    "border-destructive/60 bg-destructive/10 text-muted-foreground line-through",
  no_show: "border-destructive bg-destructive/10 text-muted-foreground",
};

/**
 * Time grid of the calendar. Day view: one column per person / resource /
 * branch. Week view: one column per day. Working time is light, the rest
 * shaded, leave and closures striped. Clicking free space starts a booking;
 * dragging a visit asks to move it (the API checks the new time).
 */
export function CalendarGrid({
  calendar,
  view,
  date,
  columnKey,
  canWrite,
  onItemClick,
  onEmptyClick,
  onMove,
}: CalendarGridProps) {
  const { t, locale } = useLocale();
  const zone = calendar.timezone;
  const drag = useRef<{ item: CalendarItem; grab: number } | null>(null);

  const placed = useMemo(
    () =>
      calendar.items.map((item) => {
        const start = zoned(item.starts_at, zone);
        const end = zoned(item.ends_at, zone);
        return {
          item,
          date: start.date,
          start: start.minutes,
          // A visit running past midnight ends at the bottom of its day.
          end: end.date === start.date ? end.minutes : 24 * 60,
        };
      }),
    [calendar.items, zone],
  );

  // Rows from the earliest opening / visit to the latest, whole hours.
  const [first, last] = useMemo(() => {
    let lo = Infinity;
    let hi = -Infinity;
    const take = (list: CalendarInterval[]) => {
      for (const iv of list) {
        const s = zoned(iv.starts_at, zone);
        const e = zoned(iv.ends_at, zone);
        lo = Math.min(lo, s.minutes);
        hi = Math.max(hi, e.date === s.date ? e.minutes : 24 * 60);
      }
    };
    take(calendar.open);
    for (const column of calendar.columns) take(column.work);
    for (const p of placed) {
      lo = Math.min(lo, p.start);
      hi = Math.max(hi, p.end);
    }
    if (!Number.isFinite(lo)) return [8 * 60, 20 * 60];
    return [
      Math.floor(lo / 60) * 60,
      Math.min(24 * 60, Math.ceil(hi / 60) * 60),
    ];
  }, [calendar.columns, calendar.open, placed, zone]);

  const height = ((last - first) / STEP) * SLOT;
  const top = (minutes: number) => ((minutes - first) / STEP) * SLOT;
  const hours: number[] = [];
  for (let m = first; m < last; m += 60) hours.push(m);

  const columnByKey = useMemo(
    () => new Map(calendar.columns.map((c) => [c.key, c])),
    [calendar.columns],
  );
  const days = dateRange(calendar.from, calendar.to);
  const now = zoned(new Date(), zone);

  type Cell = {
    key: string;
    label: string;
    sub?: string;
    color?: string | null;
    date: string;
    column: CalendarColumn | null;
    work: CalendarInterval[] | null;
    off: CalendarInterval[];
    items: CalendarItem[];
  };

  const filterColumn = columnKey === "all" ? null : columnByKey.get(columnKey);
  const cells: Cell[] =
    view === "day"
      ? calendar.columns.map((column) => ({
          key: column.key,
          label:
            column.kind === "unassigned"
              ? t("reservations.calendar.unassigned")
              : column.name,
          color: column.color,
          date,
          column,
          work: column.work,
          off: column.off,
          items: placed
            .filter(
              (p) => p.date === date && p.item.columns.includes(column.key),
            )
            .map((p) => p.item),
        }))
      : days.map((day) => ({
          key: day,
          label: new Intl.DateTimeFormat(locale, {
            weekday: "short",
            timeZone: "UTC",
          }).format(new Date(`${day}T00:00:00Z`)),
          sub: day.slice(8, 10) + "." + day.slice(5, 7),
          date: day,
          column: filterColumn ?? null,
          work: filterColumn ? filterColumn.work : calendar.open,
          off: filterColumn ? filterColumn.off : [],
          items: placed
            .filter(
              (p) =>
                p.date === day &&
                (!filterColumn || p.item.columns.includes(filterColumn.key)),
            )
            .map((p) => p.item),
        }));

  const pointAt = (cell: Cell, clientY: number, target: HTMLElement) => {
    const y = clientY - target.getBoundingClientRect().top;
    const minutes = first + Math.max(0, Math.floor(y / SLOT)) * STEP;
    return { date: cell.date, minutes, column: cell.column };
  };

  const spans = (list: CalendarInterval[], day: string) =>
    list.flatMap((iv) => {
      const s = zoned(iv.starts_at, zone);
      const e = zoned(iv.ends_at, zone);
      if (s.date > day || e.date < day) return [];
      const start = s.date < day ? first : Math.max(first, s.minutes);
      const end = e.date > day ? last : Math.min(last, e.minutes);
      return end > start ? [{ start, end }] : [];
    });

  const minWidth = view === "day" ? 150 : 120;

  return (
    <div className="bg-card overflow-auto rounded-md border">
      <div
        className="grid"
        style={{
          gridTemplateColumns: `3.5rem repeat(${cells.length}, minmax(${minWidth}px, 1fr))`,
        }}
      >
        <div className="bg-card sticky top-0 left-0 z-30 border-b" />
        {cells.map((cell) => (
          <div
            key={cell.key}
            className="bg-card sticky top-0 z-20 flex min-w-0 items-center gap-2 border-b border-l px-2 py-2 text-sm font-medium"
          >
            {cell.color ? (
              <span
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: cell.color }}
              />
            ) : null}
            <span className="truncate">{cell.label}</span>
            {cell.sub ? (
              <span className="text-muted-foreground tabular-nums">
                {cell.sub}
              </span>
            ) : null}
          </div>
        ))}

        <div className="bg-card sticky left-0 z-10 border-r" style={{ height }}>
          {hours.map((m) => (
            <div
              key={m}
              className="text-muted-foreground absolute -translate-y-2 pe-1 text-end text-xs tabular-nums"
              style={{ top: top(m), width: "3.5rem" }}
            >
              {m > first ? clock(m) : ""}
            </div>
          ))}
        </div>

        {cells.map((cell) => (
          <div
            key={cell.key}
            role="presentation"
            className="bg-muted/60 relative border-l"
            style={{ height }}
            onClick={(event: MouseEvent<HTMLDivElement>) => {
              if (!onEmptyClick || event.target !== event.currentTarget) {
                return;
              }
              onEmptyClick(pointAt(cell, event.clientY, event.currentTarget));
            }}
            onDragOver={(event: DragEvent<HTMLDivElement>) => {
              if (drag.current) event.preventDefault();
            }}
            onDrop={(event: DragEvent<HTMLDivElement>) => {
              const current = drag.current;
              drag.current = null;
              if (!current || !onMove) return;
              event.preventDefault();
              const point = pointAt(
                cell,
                event.clientY,
                event.currentTarget as HTMLElement,
              );
              onMove(current.item, {
                ...point,
                minutes: Math.max(0, point.minutes - current.grab),
              });
            }}
          >
            {(cell.work ? spans(cell.work, cell.date) : []).map((s, i) => (
              <div
                key={`w${i}`}
                className="bg-card pointer-events-none absolute inset-x-0"
                style={{ top: top(s.start), height: top(s.end) - top(s.start) }}
              />
            ))}
            {spans(cell.off, cell.date).map((s, i) => (
              <div
                key={`o${i}`}
                className="pointer-events-none absolute inset-x-0 bg-[repeating-linear-gradient(135deg,var(--muted)_0,var(--muted)_6px,transparent_6px,transparent_12px)]"
                style={{ top: top(s.start), height: top(s.end) - top(s.start) }}
                title={t("reservations.calendar.off")}
              />
            ))}
            {hours.map((m) => (
              <div
                key={m}
                className="border-border/60 pointer-events-none absolute inset-x-0 border-t"
                style={{ top: top(m) }}
              />
            ))}
            {cell.date === now.date &&
            now.minutes >= first &&
            now.minutes <= last ? (
              <div
                className="bg-destructive pointer-events-none absolute inset-x-0 z-10 h-0.5"
                style={{ top: top(now.minutes) }}
              />
            ) : null}
            {layout(cell.items).map(({ item, lane, lanes }) => {
              const start = zoned(item.starts_at, zone).minutes;
              const endZ = zoned(item.ends_at, zone);
              const end = endZ.date === cell.date ? endZ.minutes : 24 * 60;
              const movable = canWrite && canReschedule(item.status);
              const color =
                view === "day"
                  ? cell.color
                  : columnByKey.get(item.columns[0])?.color;
              return (
                <button
                  key={`${item.reservation_uuid}-${item.line_index}`}
                  type="button"
                  draggable={movable}
                  onDragStart={(event) => {
                    const rect = event.currentTarget.getBoundingClientRect();
                    drag.current = {
                      item,
                      grab:
                        Math.floor((event.clientY - rect.top) / SLOT) * STEP,
                    };
                    event.dataTransfer.effectAllowed = "move";
                    event.dataTransfer.setData(
                      "text/plain",
                      item.reservation_uuid,
                    );
                  }}
                  onDragEnd={() => {
                    drag.current = null;
                  }}
                  onClick={() => onItemClick(item)}
                  className={cn(
                    "absolute z-[5] flex flex-col overflow-hidden rounded-md border border-s-4 px-1.5 py-0.5 text-start text-xs shadow-xs transition-shadow hover:shadow-md focus-visible:ring-2 focus-visible:outline-none",
                    ITEM_TONE[item.status] ?? ITEM_TONE.active,
                    movable && "cursor-grab active:cursor-grabbing",
                  )}
                  style={{
                    top: top(Math.max(first, start)) + 1,
                    height: Math.max(SLOT * 1.5, top(end) - top(start) - 2),
                    left: `calc(${(lane / lanes) * 100}% + 2px)`,
                    width: `calc(${100 / lanes}% - 4px)`,
                    borderInlineStartColor:
                      item.status === "active" && color ? color : undefined,
                  }}
                  title={`${clock(start)}–${clock(end)} · ${item.customer_name} · ${item.service_name} · ${statusLabel(t, item.status)}`}
                >
                  <span className="flex items-center gap-1 font-medium tabular-nums">
                    {clock(start)}
                    {item.confirmation_status === "pending" ? (
                      <BellRing
                        className="size-3 text-sky-600"
                        aria-label={t("reservations.confirmation.pending")}
                      />
                    ) : null}
                  </span>
                  <span className="truncate font-medium">
                    {item.customer_name}
                    {item.party_size != null
                      ? ` · ${t("reservations.party_count", { count: item.party_size })}`
                      : ""}
                  </span>
                  <span className="truncate opacity-80">
                    {item.service_name}
                    {view === "week" && item.staff_name
                      ? ` · ${item.staff_name}`
                      : ""}
                  </span>
                </button>
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { realtimeConfig } from "@/config/realtime";
import {
  RealtimeChannels,
  parseResourceChanged,
  realtimeManager,
  type RealtimeConnectionState,
  type RealtimeMessage,
  type ResourceChangedEvent,
} from "@/lib/realtime";

/** Opt-in live updates for a server list (`resource:{name}` channel). */
export type ListRealtimeOptions = {
  /** Resource slug the backend publishes for (e.g. `platform.users`). */
  resource: string;
  /** Channel override (host-scoped channels); defaults to `resource:{resource}`. */
  channel?: string;
};

export type DataTableRealtime = ListRealtimeOptions & {
  /**
   * Silent refetch of the current list query. Page, sort, filters and
   * selection live outside the query, so they are kept.
   */
  refetch: () => unknown;
  /** Set false to pause the subscription (e.g. while a form is open). */
  enabled?: boolean;
};

export type LivePending = {
  /** Rows created elsewhere that this view does not show yet. */
  created: number;
  /** Other changes that may affect this view (bulk jobs, paused updates). */
  changed: boolean;
};

const DEBOUNCE_MS = 400;
const FADE_MS = 300;
const DROPPED_STATES: ReadonlySet<RealtimeConnectionState> = new Set([
  "disconnected",
  "reconnecting",
  "error",
  "offline",
]);

type UseLiveRowsOptions = {
  realtime?: DataTableRealtime;
  /** Ids of the rows currently on screen (DataTable row ids = uuids). */
  rowIds: string[];
  /** 0-based page index of the view. */
  pageIndex: number;
  /** True when the user scrolled away from the top of the list. */
  isScrolled: () => boolean;
  /** Defer refetches (reorder / inline edit) and surface them as a banner. */
  paused?: boolean;
  /** Called with ids that disappeared (e.g. to drop them from selection). */
  onRowsDeleted?: (ids: string[]) => void;
};

/**
 * Subscribes a list to resource-change events and decides, per debounced
 * batch, whether to refetch silently, fade out deleted rows, or show the
 * "N new records" banner instead of moving the user's view.
 */
export function useLiveRows({
  realtime,
  rowIds,
  pageIndex,
  isScrolled,
  paused = false,
  onRowsDeleted,
}: UseLiveRowsOptions) {
  const [pending, setPending] = useState<LivePending>({
    created: 0,
    changed: false,
  });
  const [fadingIds, setFadingIds] = useState<ReadonlySet<string>>(
    () => new Set(),
  );

  const resource = realtime?.resource;
  const channel =
    realtime?.channel ?? (resource ? RealtimeChannels.resource(resource) : "");
  const active =
    realtimeConfig.enabled &&
    Boolean(resource && channel) &&
    realtime?.enabled !== false;

  // Latest values for the debounced flush (no re-subscribe on every render).
  const latest = useRef({
    rowIds,
    pageIndex,
    isScrolled,
    paused,
    onRowsDeleted,
    refetch: realtime?.refetch,
  });
  useEffect(() => {
    latest.current = {
      rowIds,
      pageIndex,
      isScrolled,
      paused,
      onRowsDeleted,
      refetch: realtime?.refetch,
    };
  });

  const buffer = useRef<ResourceChangedEvent[]>([]);
  const forceRefetch = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const fadeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const refetchNow = useCallback(async () => {
    try {
      await latest.current.refetch?.();
    } catch {
      // The list shows its own error state; live updates stay fail-soft.
    }
  }, []);

  const flush = useCallback(() => {
    timer.current = null;
    const events = buffer.current;
    buffer.current = [];
    const reconnected = forceRefetch.current;
    forceRefetch.current = false;

    const {
      rowIds: ids,
      pageIndex: page,
      isScrolled: scrolled,
      paused: isPaused,
    } = latest.current;
    const visible = new Set(ids);
    const deleted = new Set<string>();
    let refetch = reconnected;
    let created = 0;
    let offscreen = false;

    for (const ev of events) {
      // A bulk event without uuids means "an unknown set of rows changed".
      const unknownSet =
        ev.truncated || (ev.action === "bulk" && ev.uuids.length === 0);
      const touched = unknownSet || ev.uuids.some((id) => visible.has(id));
      switch (ev.action) {
        case "created": {
          const fresh = ev.uuids.filter((id) => !visible.has(id)).length;
          created += ev.uuids.length ? fresh : 1;
          break;
        }
        case "deleted":
          for (const id of ev.uuids) if (visible.has(id)) deleted.add(id);
          if (touched) refetch = true;
          break;
        case "bulk":
          if (touched) refetch = true;
          else offscreen = true;
          break;
        default:
          if (touched) refetch = true;
      }
    }

    // New rows (or a bulk job that may have added some) land on page 1
    // under the default sort: refresh in place only when the user is
    // looking at the top of page 1, otherwise offer the banner.
    if (created > 0 || offscreen) {
      if (page === 0 && !scrolled() && !isPaused) {
        refetch = true;
      } else {
        setPending((prev) => ({
          created: prev.created + created,
          changed: prev.changed || offscreen,
        }));
      }
    }

    if (isPaused) {
      if (refetch) setPending((prev) => ({ ...prev, changed: true }));
      return;
    }

    if (deleted.size > 0) {
      const gone = [...deleted];
      setFadingIds((prev) => new Set([...prev, ...gone]));
      latest.current.onRowsDeleted?.(gone);
      if (fadeTimer.current) clearTimeout(fadeTimer.current);
      fadeTimer.current = setTimeout(() => {
        fadeTimer.current = null;
        void refetchNow().finally(() => setFadingIds(new Set()));
      }, FADE_MS);
      return;
    }

    if (refetch) void refetchNow();
  }, [refetchNow]);

  const schedule = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(flush, DEBOUNCE_MS);
  }, [flush]);

  // Channel subscription.
  useEffect(() => {
    if (!active || !resource) return;
    let cancelled = false;
    let unsubscribe: (() => void) | undefined;

    const onMessage = (message: RealtimeMessage) => {
      const ev = parseResourceChanged(message);
      if (!ev || ev.resource !== resource) return;
      buffer.current.push(ev);
      schedule();
    };

    realtimeManager
      .subscribe(channel, onMessage)
      .then((unsub) => {
        if (cancelled) {
          unsub();
          return;
        }
        unsubscribe = unsub;
      })
      .catch(() => {
        // Forbidden channel / realtime unavailable: the list still works.
      });

    return () => {
      cancelled = true;
      unsubscribe?.();
    };
  }, [active, channel, resource, schedule]);

  // One refetch after the connection comes back from a drop (events sent
  // while offline are not replayed).
  useEffect(() => {
    if (!active) return;
    let connected = false;
    let dropped = false;
    return realtimeManager.subscribeStatus((state) => {
      if (state === "connected") {
        if (connected && dropped) {
          forceRefetch.current = true;
          schedule();
        }
        connected = true;
        dropped = false;
      } else if (connected && DROPPED_STATES.has(state)) {
        dropped = true;
      }
    });
  }, [active, schedule]);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
      if (fadeTimer.current) clearTimeout(fadeTimer.current);
    },
    [],
  );

  // Moving to another page loads fresh rows: the banner no longer applies.
  const [prevPage, setPrevPage] = useState(pageIndex);
  if (prevPage !== pageIndex) {
    setPrevPage(pageIndex);
    if (pending.created > 0 || pending.changed) {
      setPending({ created: 0, changed: false });
    }
  }

  const dismiss = useCallback(() => {
    setPending({ created: 0, changed: false });
  }, []);

  /** Clears the banner and refetches (caller moves to page 1 / top). */
  const applyPending = useCallback(() => {
    setPending({ created: 0, changed: false });
    void refetchNow();
  }, [refetchNow]);

  return {
    active,
    pending,
    hasPending: active && (pending.created > 0 || pending.changed),
    fadingIds,
    dismiss,
    applyPending,
  };
}

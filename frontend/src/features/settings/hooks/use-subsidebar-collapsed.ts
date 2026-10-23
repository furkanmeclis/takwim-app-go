"use client";

import { useCallback, useSyncExternalStore } from "react";

const STORAGE_KEY = "app.settings.subsidebar.v1";
const CHANGE_EVENT = "app:settings-subsidebar-change";

function read(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "collapsed";
  } catch {
    return false;
  }
}

function subscribe(onStoreChange: () => void) {
  const handleStorage = (event: StorageEvent) => {
    if (event.key === STORAGE_KEY || event.key === null) onStoreChange();
  };
  window.addEventListener("storage", handleStorage);
  window.addEventListener(CHANGE_EVENT, onStoreChange);
  return () => {
    window.removeEventListener("storage", handleStorage);
    window.removeEventListener(CHANGE_EVENT, onStoreChange);
  };
}

/** Icon-only mode for the settings sub-sidebar, persisted per browser. */
export function useSubSidebarCollapsed() {
  const collapsed = useSyncExternalStore(subscribe, read, () => false);

  const toggle = useCallback(() => {
    try {
      window.localStorage.setItem(
        STORAGE_KEY,
        read() ? "expanded" : "collapsed",
      );
    } catch {
      // Storage unavailable (private mode): the toggle is a no-op.
    }
    window.dispatchEvent(new Event(CHANGE_EVENT));
  }, []);

  return [collapsed, toggle] as const;
}

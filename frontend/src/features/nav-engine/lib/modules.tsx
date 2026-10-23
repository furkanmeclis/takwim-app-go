"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from "react";

/**
 * Visible panel modules of the current shell. The tenant shell provides the
 * sector's modules (TEC-882); outside a provider (platform shell) every
 * module is visible. `null` modules mean "not known yet": module-bound
 * entries stay hidden until the scope loads, so a hidden module never
 * flashes in the sidebar.
 */
type NavModulesValue = {
  modules: readonly string[] | null;
  /** Modules the shell puts first inside their group. */
  highlights: readonly string[];
};

const NavModulesContext = createContext<NavModulesValue | null>(null);

const NO_HIGHLIGHTS: readonly string[] = [];

export function NavModulesProvider({
  modules,
  highlights = NO_HIGHLIGHTS,
  children,
}: {
  modules: readonly string[] | null;
  highlights?: readonly string[];
  children: ReactNode;
}) {
  const value = useMemo(() => ({ modules, highlights }), [modules, highlights]);
  return (
    <NavModulesContext.Provider value={value}>
      {children}
    </NavModulesContext.Provider>
  );
}

/** `hasModule(code)` for the current shell. */
export function useNavModules(): (module: string) => boolean {
  const ctx = useContext(NavModulesContext);
  const modules = ctx?.modules;
  const scoped = ctx !== null;
  return useCallback(
    (module: string) => !scoped || Boolean(modules?.includes(module)),
    [modules, scoped],
  );
}

/** Highlighted modules of the current shell (empty outside a provider). */
export function useNavHighlights(): readonly string[] {
  return useContext(NavModulesContext)?.highlights ?? NO_HIGHLIGHTS;
}

/**
 * Highlighted module entries first (in highlight order), the rest in
 * catalog order.
 */
export function orderByHighlights<T extends { module?: string }>(
  items: T[],
  highlights: readonly string[],
): T[] {
  if (!highlights.length) return items;
  const rank = (item: T) => {
    const i = item.module ? highlights.indexOf(item.module) : -1;
    return i === -1 ? highlights.length : i;
  };
  return items
    .map((item, index) => ({ item, index }))
    .sort((a, b) => rank(a.item) - rank(b.item) || a.index - b.index)
    .map(({ item }) => item);
}

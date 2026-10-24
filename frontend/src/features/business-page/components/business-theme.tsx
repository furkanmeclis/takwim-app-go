"use client";

import { useMemo, type ReactNode } from "react";

import {
  resolveTheme,
  themeScope,
  themeStylesheet,
  type BusinessTheme as BusinessThemeValues,
  type ColorMode,
} from "@/features/business-page/lib/theme";
import { themeFontProps } from "@/features/business-page/lib/theme-fonts";
import { cn } from "@/lib/utils";

/**
 * Applies a business's theme (TEC-966) to everything inside: the colours
 * become CSS variables scoped to this element (light values, dark ones under
 * the app's `.dark` class) and the font switches through next/font classes.
 * Colours that fail the contrast check stay neutral in that mode, so the
 * wrapper renders the plain app look when the theme is empty or unreadable.
 * `mode` forces one colour mode (the panel preview).
 */
export function BusinessTheme({
  scope,
  theme,
  mode = "auto",
  className,
  children,
}: {
  scope: string;
  theme: BusinessThemeValues | undefined;
  mode?: ColorMode | "auto";
  className?: string;
  children: ReactNode;
}) {
  const id = themeScope(scope);
  const resolved = useMemo(() => resolveTheme(theme), [theme]);
  const css = useMemo(
    () => themeStylesheet(id, resolved, mode),
    [id, resolved, mode],
  );
  const font = themeFontProps(resolved.font);
  return (
    <div
      data-business-theme={id}
      className={cn(font.className, className)}
      style={font.style}
    >
      {css ? <style>{css}</style> : null}
      {children}
    </div>
  );
}

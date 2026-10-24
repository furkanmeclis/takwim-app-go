/**
 * Business page theme (TEC-966): the colours a business picks in the panel
 * become CSS variables of the page and the embedded screen, after a
 * contrast check per colour mode. A colour that cannot be read falls back
 * to the app's neutral theme in that mode, so the page stays accessible
 * whatever the business saves. Pure functions; shared by the page and the
 * panel preview.
 */

import type { PublicBusiness } from "@/features/business-page/types";

export type BusinessTheme = PublicBusiness["theme"];
export type ThemeFont = BusinessTheme["font"];
export type ColorMode = "light" | "dark";

/** Font options of the page, in display order (API enum). */
export const THEME_FONTS = [
  "default",
  "inter",
  "manrope",
  "nunito",
  "lora",
  "playfair_display",
] as const satisfies readonly ThemeFont[];

/** Static label keys of the fonts (no template keys, see check-i18n). */
export const THEME_FONT_LABEL_KEYS: Record<ThemeFont, string> = {
  default: "tenant_settings.theme.fonts.default",
  inter: "tenant_settings.theme.fonts.inter",
  manrope: "tenant_settings.theme.fonts.manrope",
  nunito: "tenant_settings.theme.fonts.nunito",
  lora: "tenant_settings.theme.fonts.lora",
  playfair_display: "tenant_settings.theme.fonts.playfair_display",
};

/** Theme of a business that never set one: the neutral app look. */
export const NEUTRAL_THEME: BusinessTheme = {
  primary_color: "",
  accent_color: "",
  font: "default",
  headline: "",
  description: "",
  cover_url: null,
};

export const HEX_COLOR = /^#[0-9a-fA-F]{6}$/;

/** WCAG minimum contrast of text (AA, normal text). */
const TEXT_CONTRAST = 4.5;
/** WCAG minimum contrast of a control against its surroundings (1.4.11). */
const SURFACE_CONTRAST = 3;

/**
 * Backgrounds and foregrounds of the two colour modes (styles/theme.css in
 * sRGB), used only for the contrast checks.
 */
const MODE_SURFACES: Record<ColorMode, { background: Rgb; foreground: Rgb }> = {
  light: { background: [255, 255, 255], foreground: [30, 41, 59] },
  dark: { background: [15, 23, 42], foreground: [248, 250, 252] },
};

type Rgb = [number, number, number];

export function parseHex(value: string | null | undefined): Rgb | null {
  if (!value || !HEX_COLOR.test(value)) return null;
  const n = Number.parseInt(value.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function channel(v: number) {
  const c = v / 255;
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/** Relative luminance (WCAG 2.x). */
export function luminance([r, g, b]: Rgb) {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** Contrast ratio between two colours (1 to 21). */
export function contrastRatio(a: Rgb, b: Rgb) {
  const la = luminance(a);
  const lb = luminance(b);
  const [hi, lo] = la > lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

function toHex([r, g, b]: Rgb) {
  return (
    "#" + [r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")
  ).toLowerCase();
}

const WHITE: Rgb = [255, 255, 255];
const INK: Rgb = [15, 23, 42];

/** CSS variables of one colour mode; absent = the neutral value stays. */
export type ThemeVars = Partial<
  Record<
    | "--primary"
    | "--primary-foreground"
    | "--ring"
    | "--accent"
    | "--accent-foreground",
    string
  >
>;

export type ResolvedMode = {
  vars: ThemeVars;
  /** Colours dropped in this mode because they could not be read. */
  fallbacks: ("primary_color" | "accent_color")[];
};

/**
 * Resolves the colours of one mode. The primary colour needs a readable
 * text colour on it (white or ink) and must stand out from the page
 * background; the accent is a soft surface whose text is the mode's
 * foreground.
 */
export function resolveMode(
  theme: BusinessTheme,
  mode: ColorMode,
): ResolvedMode {
  const vars: ThemeVars = {};
  const fallbacks: ResolvedMode["fallbacks"] = [];
  const surfaces = MODE_SURFACES[mode];

  const primary = parseHex(theme.primary_color);
  if (primary) {
    const onWhite = contrastRatio(primary, WHITE);
    const onInk = contrastRatio(primary, INK);
    const foreground = onWhite >= onInk ? WHITE : INK;
    const readable = Math.max(onWhite, onInk) >= TEXT_CONTRAST;
    const visible =
      contrastRatio(primary, surfaces.background) >= SURFACE_CONTRAST;
    if (readable && visible) {
      vars["--primary"] = toHex(primary);
      vars["--primary-foreground"] = toHex(foreground);
      vars["--ring"] = toHex(primary);
    } else {
      fallbacks.push("primary_color");
    }
  }

  const accent = parseHex(theme.accent_color);
  if (accent) {
    if (contrastRatio(accent, surfaces.foreground) >= TEXT_CONTRAST) {
      vars["--accent"] = toHex(accent);
      vars["--accent-foreground"] = toHex(surfaces.foreground);
    } else {
      fallbacks.push("accent_color");
    }
  }
  return { vars, fallbacks };
}

export type ResolvedTheme = {
  light: ResolvedMode;
  dark: ResolvedMode;
  font: ThemeFont;
  /** True when nothing changes the neutral look. */
  neutral: boolean;
};

export function resolveTheme(theme: BusinessTheme | undefined): ResolvedTheme {
  const t = theme ?? NEUTRAL_THEME;
  const light = resolveMode(t, "light");
  const dark = resolveMode(t, "dark");
  const font = THEME_FONTS.includes(t.font) ? t.font : "default";
  return {
    light,
    dark,
    font,
    neutral:
      Object.keys(light.vars).length === 0 &&
      Object.keys(dark.vars).length === 0 &&
      font === "default",
  };
}

function declarations(vars: ThemeVars) {
  return Object.entries(vars)
    .map(([name, value]) => `${name}:${value};`)
    .join("");
}

/**
 * Stylesheet of a theme scoped to `[data-business-theme="<scope>"]`. Light
 * values apply by default and dark ones under the app's `.dark` class, or
 * only one mode when `mode` is forced (the panel preview). Every value is a
 * validated `#rrggbb`, so the text is safe to inline.
 */
export function themeStylesheet(
  scope: string,
  resolved: ResolvedTheme,
  mode: ColorMode | "auto" = "auto",
) {
  const selector = `[data-business-theme="${scope}"]`;
  const rules: string[] = [];
  if (mode === "auto" || mode === "light") {
    const light = declarations(resolved.light.vars);
    if (light) rules.push(`${selector}{${light}}`);
  }
  if (mode === "auto") {
    const dark = declarations(resolved.dark.vars);
    if (dark) rules.push(`.dark ${selector}{${dark}}`);
  } else if (mode === "dark") {
    const dark = declarations(resolved.dark.vars);
    if (dark) rules.push(`${selector}{${dark}}`);
  }
  return rules.join("\n");
}

/** A scope token safe for an attribute selector (slug or "preview"). */
export function themeScope(raw: string) {
  return raw.replace(/[^a-z0-9_-]/gi, "").slice(0, 64) || "page";
}

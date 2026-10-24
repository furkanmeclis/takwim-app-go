import {
  Inter,
  Lora,
  Manrope,
  Nunito,
  Playfair_Display,
} from "next/font/google";

import type { ThemeFont } from "@/features/business-page/lib/theme";

/**
 * Fonts a business may pick for its page (TEC-966). next/font self-hosts
 * them with the app, so a themed page never loads a font from a third
 * party. `default` is the app's own font (Geist, set on `<body>`).
 */
const inter = Inter({
  variable: "--font-theme-inter",
  subsets: ["latin", "latin-ext"],
  display: "swap",
});
const manrope = Manrope({
  variable: "--font-theme-manrope",
  subsets: ["latin", "latin-ext"],
  display: "swap",
});
const nunito = Nunito({
  variable: "--font-theme-nunito",
  subsets: ["latin", "latin-ext"],
  display: "swap",
});
const lora = Lora({
  variable: "--font-theme-lora",
  subsets: ["latin", "latin-ext"],
  display: "swap",
});
const playfair = Playfair_Display({
  variable: "--font-theme-playfair",
  subsets: ["latin", "latin-ext"],
  display: "swap",
});

type FontAsset = { className: string; variable: string; family: string };

const FONTS: Record<Exclude<ThemeFont, "default">, FontAsset> = {
  inter: {
    className: inter.variable,
    variable: "--font-theme-inter",
    family: "Inter",
  },
  manrope: {
    className: manrope.variable,
    variable: "--font-theme-manrope",
    family: "Manrope",
  },
  nunito: {
    className: nunito.variable,
    variable: "--font-theme-nunito",
    family: "Nunito",
  },
  lora: {
    className: lora.variable,
    variable: "--font-theme-lora",
    family: "Lora",
  },
  playfair_display: {
    className: playfair.variable,
    variable: "--font-theme-playfair",
    family: "Playfair Display",
  },
};

const FALLBACK = "ui-sans-serif, system-ui, sans-serif";

/**
 * Class and inline style of a wrapper that switches the font of everything
 * inside it: `--font-sans` (headings read it) and `font-family` (inherited
 * by text and form controls). The default font returns nothing.
 */
export function themeFontProps(font: ThemeFont): {
  className?: string;
  style?: React.CSSProperties;
} {
  if (font === "default") return {};
  const asset = FONTS[font];
  const family = `var(${asset.variable}), ${FALLBACK}`;
  return {
    className: asset.className,
    style: {
      fontFamily: family,
      ["--font-sans" as string]: family,
      ["--font-display" as string]: family,
    } as React.CSSProperties,
  };
}

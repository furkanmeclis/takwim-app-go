/**
 * App identity tokens (Takwim brand kit, see docs/brand.md).
 * Hex aliases for JS (theme-color, icons). CSS tokens live in theme.css.
 */
export const brand = {
  name: "Takwim",
  productName: "Takwim",
  tagline: "Randevu ve işletme yönetim platformu",
  /** Custom URL scheme of the companion mobile app (QR sign-in fallback). */
  mobileScheme: "takwim",
  colors: {
    /** Brand navy — calendar mark, light-mode primary. */
    primary: "#022C81",
    primaryForeground: "#FFFFFF",
    /** Brand sky — the dot, dark-mode primary (ink text on it). */
    accent: "#60A7FE",
    /** Brand ink — wordmark letters and foreground text. */
    ink: "#041F47",
    /** slate-100 */
    mist: "#F1F5F9",
    white: "#FFFFFF",
    /** Dark-mode page background (theme-color in dark mode). */
    navy: "#020617",
    lightGray: "#F1F5F9",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon" | "stacked";

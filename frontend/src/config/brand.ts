/**
 * App identity tokens. Swap name / colors when you brand the boilerplate.
 * Hex aliases for JS (theme-color, icons). CSS tokens live in theme.css.
 */
export const brand = {
  name: "App",
  productName: "Boilerplate",
  tagline: "Platform & CMS console",
  /** Custom URL scheme of the companion mobile app (QR sign-in fallback). */
  mobileScheme: "app",
  colors: {
    /** slate-900 — shadcn slate primary */
    primary: "#0F172A",
    primaryForeground: "#F8FAFC",
    /** slate-950 */
    ink: "#020617",
    /** slate-100 */
    mist: "#F1F5F9",
    white: "#FFFFFF",
    /** Compat aliases used by theme-switch / legacy tokens */
    navy: "#020617",
    lightGray: "#F1F5F9",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon";

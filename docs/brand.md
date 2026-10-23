# Takwim brand

The product name is **Takwim**. Technical names (`NAME_PREFIX`, database,
bucket, Meilisearch prefix) use lowercase `takwim`.

## Brand kit

Source SVGs live in [`frontend/src/assets/brand/`](../frontend/src/assets/brand), copied unchanged from the
official logo set. The kit's own file names were renamed so they say what they
show:

| File | Kit name | Art |
|------|----------|-----|
| `takwim-logo-colorful.svg` | `takwim-colorful-vertical.svg` | Horizontal lockup: navy calendar, sky dot, ink letters (2000×629) |
| `takwim-logo-blue.svg` / `takwim-logo-dark.svg` | `takwim-blue-vertical.svg` / `takwim-dark-vertical.svg` | Horizontal lockup for navy / dark backgrounds |
| `takwim-stacked-*.svg` | `takwim-*.svg` | Stacked lockup, mark above letters (2000×2000) |
| `takwim-icon-colorful.svg` | `takwim-colorful-icon.svg` | Mark only, transparent background |
| `takwim-icon-blue.svg` | `takwim-blue-icon.svg` | App icon: white calendar and sky dot on navy |
| `takwim-icon-dark.svg` | `takwim-dark-icon.svg` | Mark only, all white |

## Colours

| Token | Hex | Where |
|-------|-----|-------|
| Navy (primary) | `#022C81` | Calendar mark, light-mode `--primary`, `--sidebar-primary`, `--chart-1`, app icon background |
| Sky (accent) | `#60A7FE` | The dot, dark-mode `--primary`, `--sidebar-primary`, `--ring`, `--chart-1` |
| Ink | `#041F47` | Wordmark letters, light-mode `--foreground`, text on the dark-mode primary |
| Ring (light) | `#3B6FD8` | Light-mode `--ring`, `--sidebar-ring` |
| Active nav tint | `#E8F1FF` light / `#132A55` dark | `--sidebar-accent` (text `#022C81` / `#C9DEFF`) |

Contrast (WCAG 2.x):

| Pair | Ratio |
|------|-------|
| White on `#022C81` (button text, light) | 12.5:1 |
| `#022C81` on `#E8F1FF` (active nav, light) | 11.0:1 |
| `#041F47` on `#60A7FE` (button text, dark) | 6.5:1 |
| `#60A7FE` on `#020617` (links, dark background) | 8.1:1 |
| `#60A7FE` on `#0F172A` (links, dark card) | 7.2:1 |
| `#C9DEFF` on `#132A55` (active nav, dark) | 10.3:1 |

CSS tokens are in [`frontend/src/styles/theme.css`](../frontend/src/styles/theme.css); JS hex aliases in
[`frontend/src/config/brand.ts`](../frontend/src/config/brand.ts).

## Usage rules

- Light background → colourful logo. Dark or navy background → white logo.
- The app icon tile works on any background and is never recoloured.
- Don't stretch, outline, shadow or recolour the art, and don't retype the
  wordmark in a font.

## In the panel

`@/components/brand`:

| Component | Renders |
|-----------|---------|
| `AppLogo variant="logo"` | Horizontal lockup (default) |
| `AppLogo variant="stacked"` | Stacked lockup |
| `AppLogo variant="wordmark"` | Letters only |
| `AppLogo variant="mark"` / `AppMark` | Calendar glyph |
| `AppLogo variant="icon"` | App icon tile |
| `AppWordmark` | Horizontal lockup sized for the auth pages |
| `AppSidebarLogo` | Icon tile + name + tagline |

`tone` names the background the logo sits on: `light` → colourful, `dark` →
white, `auto` (default) → colourful in light mode, white in dark mode. The
paths are inline (`takwim-paths.ts`), so no request is made for the logo.
Favicon: `frontend/src/app/icon.svg`; Apple touch icon:
`frontend/src/app/apple-icon.tsx`.

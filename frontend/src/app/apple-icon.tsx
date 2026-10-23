import { ImageResponse } from "next/og";

import { TAKWIM_PATHS } from "@/components/brand/takwim-paths";
import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/**
 * Apple touch icon — the brand kit app icon: white calendar and sky dot on
 * the navy square (iOS rounds the corners itself).
 */
export default function AppleIcon() {
  const { mark, dot } = TAKWIM_PATHS.mark;
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        background: brand.colors.primary,
      }}
    >
      <svg width="180" height="180" viewBox="0 0 2000 2000">
        {mark.map((p) => (
          <path
            key={p.d.slice(0, 24)}
            fill={brand.colors.white}
            fillRule="evenodd"
            d={p.d}
            transform={p.transform}
          />
        ))}
        <circle cx={dot.cx} cy={dot.cy} r={dot.r} fill={brand.colors.accent} />
      </svg>
    </div>,
    { ...size },
  );
}

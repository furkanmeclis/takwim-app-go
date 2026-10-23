import { ImageResponse } from "next/og";

import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/** Apple touch / PWA icon — hex mark on ink. */
export default function AppleIcon() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: brand.colors.ink,
        borderRadius: 36,
      }}
    >
      <svg width="96" height="96" viewBox="0 0 24 24" fill={brand.colors.white}>
        <path d="M11.1 2.55a2 2 0 0 1 1.8 0l7.15 3.9A2 2 0 0 1 21 8.22v7.56a2 2 0 0 1-1.05 1.77l-7.15 3.9a2 2 0 0 1-1.8 0l-7.15-3.9A2 2 0 0 1 3 15.78V8.22a2 2 0 0 1 1.05-1.77l7.15-3.9Z" />
      </svg>
    </div>,
    { ...size },
  );
}

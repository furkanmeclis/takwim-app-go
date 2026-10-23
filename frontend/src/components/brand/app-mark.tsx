import type { BrandTone } from "@/config/brand";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { TakwimMarkSvg } from "./brand-svg";

type AppMarkProps = {
  title?: string;
  tone?: BrandTone;
  className?: string;
};

/** The Takwim calendar glyph (navy on light, white on dark with `auto`). */
export function AppMark({ className, title = brand.name, tone }: AppMarkProps) {
  return (
    <TakwimMarkSvg
      title={title}
      tone={tone}
      className={cn("size-4", className)}
    />
  );
}

import { brand, type BrandTone } from "@/config/brand";
import { cn } from "@/lib/utils";

import { TakwimLogoSvg } from "./brand-svg";

type AppWordmarkProps = {
  title?: string;
  tone?: BrandTone;
  className?: string;
};

/** Horizontal Takwim lockup for the auth pages. */
export function AppWordmark({
  className,
  title = brand.productName,
  tone = "auto",
}: AppWordmarkProps) {
  return (
    <TakwimLogoSvg
      title={title}
      tone={tone}
      className={cn("h-8 w-auto", className)}
    />
  );
}

"use client";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { TakwimIconSvg } from "./brand-svg";

type AppSidebarLogoProps = {
  className?: string;
};

/** Sidebar header lockup — app icon tile + product name and tagline. */
export function AppSidebarLogo({ className }: AppSidebarLogoProps) {
  const { t } = useLocale();
  return (
    <>
      <TakwimIconSvg
        title={brand.name}
        className={cn("aspect-square size-8! rounded-lg", className)}
      />
      <span className="grid flex-1 text-left text-sm leading-tight">
        <span className="truncate font-medium">{brand.productName}</span>
        <span className="truncate text-xs opacity-70">
          {t("common.app_tagline")}
        </span>
      </span>
    </>
  );
}

import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

type AppMarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/** Generic hex mark — works on primary tiles and as a standalone glyph. */
export function AppMark({
  className,
  title = brand.name,
  ...props
}: AppMarkProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("size-4", className)}
      {...props}
    >
      <title>{title}</title>
      <path
        fill="currentColor"
        d="M11.1 2.55a2 2 0 0 1 1.8 0l7.15 3.9A2 2 0 0 1 21 8.22v7.56a2 2 0 0 1-1.05 1.77l-7.15 3.9a2 2 0 0 1-1.8 0l-7.15-3.9A2 2 0 0 1 3 15.78V8.22a2 2 0 0 1 1.05-1.77l7.15-3.9Z"
      />
    </svg>
  );
}

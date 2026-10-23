import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";

type AppWordmarkProps = {
  title?: string;
  className?: string;
};

/** Icon tile + product name — shadcn login / sidebar lockup. */
export function AppWordmark({
  className,
  title = brand.productName,
}: AppWordmarkProps) {
  return (
    <span
      className={cn("inline-flex items-center gap-2 font-medium", className)}
    >
      <span className="bg-primary text-primary-foreground flex size-6 items-center justify-center rounded-md">
        <AppMark className="size-3.5" title={title} />
      </span>
      <span className="text-sm">{title}</span>
    </span>
  );
}

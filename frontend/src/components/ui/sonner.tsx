"use client";

import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
  XIcon,
} from "lucide-react";
import { useTheme } from "next-themes";
import type { ReactNode } from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";

/**
 * App toasts: Sonner keeps stacking, swipe, timers and the `toast.*`
 * API; the look is ours (`unstyled`). Each type sets `--toast-accent`, which
 * tints the icon tile (brand primary when unset).
 */
function ToastIcon({ children }: { children: ReactNode }) {
  return (
    <span className="flex size-9 items-center justify-center rounded-xl bg-[color-mix(in_oklab,var(--toast-accent,var(--primary))_14%,transparent)] text-(--toast-accent,var(--primary)) ring-1 ring-[color-mix(in_oklab,var(--toast-accent,var(--primary))_22%,transparent)]">
      {children}
    </span>
  );
}

const Toaster = ({ toastOptions, ...props }: ToasterProps) => {
  const { resolvedTheme, theme = "light" } = useTheme();
  const toastTheme = resolvedTheme ?? theme;

  return (
    <Sonner
      theme={toastTheme as ToasterProps["theme"]}
      className="toaster group"
      gap={10}
      icons={{
        success: (
          <ToastIcon>
            <CircleCheckIcon className="size-4.5" />
          </ToastIcon>
        ),
        info: (
          <ToastIcon>
            <InfoIcon className="size-4.5" />
          </ToastIcon>
        ),
        warning: (
          <ToastIcon>
            <TriangleAlertIcon className="size-4.5" />
          </ToastIcon>
        ),
        error: (
          <ToastIcon>
            <OctagonXIcon className="size-4.5" />
          </ToastIcon>
        ),
        loading: (
          <ToastIcon>
            <Loader2Icon className="size-4.5 animate-spin" />
          </ToastIcon>
        ),
        close: <XIcon className="size-3.5" />,
      }}
      toastOptions={{
        unstyled: true,
        ...toastOptions,
        classNames: {
          toast:
            "group/toast pointer-events-auto relative flex w-full items-center gap-3 rounded-2xl border border-border/60 bg-popover/85 p-3 pe-10 text-popover-foreground shadow-xl shadow-black/10 backdrop-blur-xl sm:w-(--width) dark:shadow-black/40",
          success: "[--toast-accent:oklch(0.63_0.15_158)]",
          info: "[--toast-accent:var(--primary)]",
          warning: "[--toast-accent:oklch(0.72_0.16_70)]",
          error: "[--toast-accent:var(--destructive)]",
          loading: "[--toast-accent:var(--primary)]",
          icon: "m-0! size-9! shrink-0 self-start",
          content: "flex min-w-0 flex-1 flex-col gap-0.5",
          title: "text-sm leading-5 font-semibold",
          description: "text-muted-foreground text-[13px] leading-5",
          actionButton:
            "bg-primary text-primary-foreground hover:bg-primary/90 h-8 shrink-0 rounded-lg px-3 text-xs font-medium transition-colors",
          cancelButton:
            "bg-muted text-muted-foreground hover:bg-muted/80 h-8 shrink-0 rounded-lg px-3 text-xs font-medium transition-colors",
          closeButton:
            "text-muted-foreground hover:bg-muted hover:text-foreground absolute end-2.5 top-2.5 flex size-6 items-center justify-center rounded-md transition-colors",
          ...toastOptions?.classNames,
        },
      }}
      {...props}
    />
  );
};

export { Toaster };

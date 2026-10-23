"use client";

import { RefreshCw, X } from "lucide-react";

import type { LivePending } from "@/components/tables/use-live-rows";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

type LiveUpdatesBannerProps = {
  pending: LivePending;
  onRefresh: () => void;
  onDismiss: () => void;
};

/**
 * Compact notice for live list changes the user is not looking at
 * ("3 new records – Refresh"). Same look as the bulk selection banner.
 */
export function LiveUpdatesBanner({
  pending,
  onRefresh,
  onDismiss,
}: LiveUpdatesBannerProps) {
  const { t } = useLocale();

  if (pending.created <= 0 && !pending.changed) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="bg-muted/50 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm"
    >
      <RefreshCw className="text-muted-foreground size-3.5" aria-hidden />
      <span className="text-muted-foreground">
        {pending.created === 1
          ? t("table.live_new_record", { count: 1 })
          : pending.created > 1
            ? t("table.live_new_records", { count: pending.created })
            : t("table.live_changed")}
      </span>
      <Button
        type="button"
        size="sm"
        variant="link"
        className="h-auto p-0"
        onClick={onRefresh}
      >
        {t("table.live_refresh")}
      </Button>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        className="text-muted-foreground ms-auto size-6"
        aria-label={t("table.live_dismiss")}
        onClick={onDismiss}
      >
        <X className="size-3.5" />
      </Button>
    </div>
  );
}

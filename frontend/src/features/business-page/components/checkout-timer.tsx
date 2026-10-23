"use client";

import { Clock } from "lucide-react";
import { useEffect, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Progress } from "@/components/ui/progress";
import { useLocale } from "@/providers/locale-provider";

function useNow(intervalMs: number) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);
  return now;
}

/**
 * Countdown of the checkout hold. When it runs out the time is no longer
 * kept for the customer, but the booking still goes through while the
 * time is free (the API books it again in one transaction).
 */
export function CheckoutTimer({
  expiresAt,
  totalMinutes,
}: {
  expiresAt: string | null;
  totalMinutes: number;
}) {
  const { t } = useLocale();
  const now = useNow(1000);
  if (!expiresAt) return null;
  const left = Math.max(0, Date.parse(expiresAt) - now);
  const total = Math.max(1, totalMinutes) * 60_000;

  if (left === 0) {
    return (
      <Alert>
        <Clock />
        <AlertTitle>{t("business_page.hold.expired_title")}</AlertTitle>
        <AlertDescription>
          {t("business_page.hold.expired_description")}
        </AlertDescription>
      </Alert>
    );
  }

  const seconds = Math.ceil(left / 1000);
  const clock = `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
  return (
    <div
      className="bg-card space-y-2 rounded-lg border p-3"
      role="timer"
      aria-live="off"
    >
      <p className="flex items-center gap-2 text-sm">
        <Clock className="text-muted-foreground size-4" />
        <span>{t("business_page.hold.held_for")}</span>
        <span className="ms-auto font-semibold tabular-nums">{clock}</span>
      </p>
      <Progress value={(left / total) * 100} />
    </div>
  );
}

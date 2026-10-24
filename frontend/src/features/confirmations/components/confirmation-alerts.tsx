"use client";

import { BellRing } from "lucide-react";
import Link from "next/link";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  useConfirmationFollowUpAccess,
  useConfirmationSummary,
} from "@/features/confirmations/hooks/use-confirmations";
import type { ConfirmationSummary } from "@/features/confirmations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const REASONS: {
  key: keyof Omit<ConfirmationSummary, "attention" | "waiting">;
  label: string;
}[] = [
  { key: "change_requested", label: "confirmations.alerts.change_requested" },
  { key: "unreachable", label: "confirmations.alerts.unreachable" },
  { key: "flagged", label: "confirmations.alerts.flagged" },
  { key: "send_failed", label: "confirmations.alerts.send_failed" },
];

/**
 * The follow-up alert of the panel (TEC-924): upcoming visits whose
 * confirmation needs a person before the visit (a change request, no
 * answer, no way to reach the customer, a request that failed on every
 * channel). Hidden without the add-on, without the reservations read or
 * when nothing needs a person. `onShow` filters the list in place;
 * without it the button opens the confirmation list.
 */
export function ConfirmationAlerts({ onShow }: { onShow?: () => void }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const access = useConfirmationFollowUpAccess();
  const summary = useConfirmationSummary(access);
  const data = summary.data;
  if (!access || !data || data.attention === 0) return null;

  return (
    <Alert>
      <BellRing className="size-4" />
      <AlertTitle>
        {t("confirmations.alerts.title", { count: data.attention })}
      </AlertTitle>
      <AlertDescription className="space-y-3">
        <p>{t("confirmations.alerts.description")}</p>
        <div className="flex flex-wrap gap-1.5">
          {REASONS.filter((reason) => data[reason.key] > 0).map((reason) => (
            <Badge key={reason.key} variant="outline">
              {t(reason.label, { count: data[reason.key] })}
            </Badge>
          ))}
        </div>
        {onShow ? (
          <Button size="sm" variant="outline" onClick={onShow}>
            {t("confirmations.alerts.show")}
          </Button>
        ) : (
          <Button asChild size="sm" variant="outline">
            <Link
              href={`${routes.tenant.reservations.confirmations(slug)}?view=attention`}
            >
              {t("confirmations.alerts.show")}
            </Link>
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}

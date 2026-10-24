"use client";

import { Loader2 } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  useCustomerAvailability,
  useReloadReservation,
  useRescheduleReservation,
} from "@/features/customer-panel/hooks/use-customer-panel";
import {
  customerErrorMessage,
  isConflict,
} from "@/features/customer-panel/lib/errors";
import type { CustomerReservation } from "@/features/customer-panel/types";
import { dateKeyInZone, datetimeInZone } from "@/lib/utils/format";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

const DAY_STATUS_KEYS: Record<string, string> = {
  closed: "customer_panel.reschedule.closed",
  too_far: "customer_panel.reschedule.too_far",
  too_soon: "customer_panel.reschedule.too_soon",
  no_class: "customer_panel.reschedule.no_class",
  no_staff: "customer_panel.reschedule.full",
  full: "customer_panel.reschedule.full",
};

/**
 * Moves the visit to another free start of a day: same services, same
 * people, the business's online booking rules. The API checks the change
 * deadline and the slot again; the old time is let go only when the new
 * one is taken.
 */
export function CustomerRescheduleDialog({
  reservation,
  open,
  onOpenChange,
}: {
  reservation: CustomerReservation;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const zone = reservation.timezone;
  const [date, setDate] = useState(() =>
    dateKeyInZone(reservation.starts_at, zone),
  );
  const [startsAt, setStartsAt] = useState("");
  const availability = useCustomerAvailability(reservation.uuid, date, open);
  const reschedule = useRescheduleReservation(reservation.uuid);
  const reload = useReloadReservation(reservation.uuid);
  const day = availability.data?.days[0];
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });

  const submit = () => {
    if (!startsAt) return;
    reschedule.mutate(
      { startsAt, version: reservation.version },
      {
        onSuccess: () => {
          appToast.success(t("customer_panel.reschedule.done"));
          setStartsAt("");
          onOpenChange(false);
        },
        onError: (error) => {
          appToast.error(customerErrorMessage(t, error));
          setStartsAt("");
          if (isConflict(error)) {
            void reload();
            void availability.refetch();
          }
        },
      },
    );
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      className="sm:max-w-xl"
      title={t("customer_panel.actions.reschedule")}
      description={t("customer_panel.reschedule.description")}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("customer_panel.actions.close")}
          </Button>
          <Button
            type="button"
            disabled={!startsAt || reschedule.isPending}
            onClick={submit}
          >
            {t("customer_panel.reschedule.submit")}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <p className="text-muted-foreground text-sm">
          {t("customer_panel.reschedule.current")}:{" "}
          <span className="text-foreground tabular-nums">
            {datetimeInZone(reservation.starts_at, zone, locale, {
              dateStyle: "medium",
              timeStyle: "short",
            })}
          </span>
        </p>
        <DatePicker
          value={date}
          onChange={(next) => {
            setDate(next);
            setStartsAt("");
          }}
        />
        {availability.isLoading ? (
          <p className="text-muted-foreground flex items-center gap-2 text-sm">
            <Loader2 className="size-4 animate-spin" />
            {t("customer_panel.reschedule.loading")}
          </p>
        ) : availability.isError ? (
          <ErrorState
            title={customerErrorMessage(t, availability.error)}
            onRetry={() => void availability.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : day && day.starts.length > 0 ? (
          <ToggleGroup
            type="single"
            variant="outline"
            spacing={2}
            value={startsAt}
            onValueChange={setStartsAt}
            className="flex flex-wrap justify-start gap-2"
            aria-label={t("customer_panel.reschedule.slot")}
          >
            {day.starts.map((start) => (
              <ToggleGroupItem
                key={start}
                value={start}
                className="min-w-16 tabular-nums"
              >
                {time(start)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        ) : (
          <EmptyState
            className="py-8"
            title={t(
              DAY_STATUS_KEYS[day?.status ?? "full"] ??
                "customer_panel.reschedule.full",
            )}
            description={t("customer_panel.reschedule.try_other_day")}
          />
        )}
      </div>
    </AppDialog>
  );
}

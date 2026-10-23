"use client";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import type { GridPoint } from "@/features/reservations/components/calendar/calendar-grid";
import {
  useReservation,
  useRescheduleReservation,
} from "@/features/reservations/hooks/use-reservations";
import {
  clock,
  daysBetween,
  zoned,
} from "@/features/reservations/lib/calendar-time";
import type { CalendarItem, Reservation } from "@/features/reservations/types";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type MoveTarget = {
  item: CalendarItem;
  point: GridPoint;
};

/**
 * The request of a drop: the visit keeps its shape (every line moves by the
 * same amount); dropped on another person's column, the dragged line goes
 * to that person. Resources are planned again by the API.
 */
export function moveRequest(
  reservation: Reservation,
  target: MoveTarget,
  zone: string,
) {
  const from = zoned(target.item.starts_at, zone);
  const shift =
    daysBetween(from.date, target.point.date) * 1440 +
    target.point.minutes -
    from.minutes;
  const startsAt = new Date(
    Date.parse(reservation.starts_at) + shift * 60_000,
  ).toISOString();
  const column = target.point.column;
  const line = reservation.lines.find(
    (l) => l.index === target.item.line_index,
  );
  const newStaff =
    column?.kind === "staff" && line && line.staff?.uuid !== column.key
      ? column
      : null;
  const editable = reservation.lines.every((l) => l.service_uuid);
  return {
    startsAt,
    shift,
    staff: newStaff && editable ? newStaff : null,
    lines:
      newStaff && editable
        ? reservation.lines.map((l) => ({
            service_uuid: l.service_uuid as string,
            staff_uuid:
              l.index === target.item.line_index
                ? newStaff.key
                : (l.staff?.uuid ?? null),
          }))
        : undefined,
  };
}

/** Confirms a drag on the calendar; the API checks the new time. */
export function MoveDialog({
  target,
  zone,
  onOpenChange,
}: {
  target: MoveTarget | null;
  zone: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const query = useReservation(target?.item.reservation_uuid ?? null);
  const reschedule = useRescheduleReservation();
  const reservation = query.data;
  const plan =
    reservation && target ? moveRequest(reservation, target, zone) : null;

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={onOpenChange}
      title={
        target
          ? `${t("reservations.actions.reschedule")} · #${target.item.number}`
          : ""
      }
      description={t("reservations.calendar.move_description")}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={
              !plan || (plan.shift === 0 && !plan.staff) || reschedule.isPending
            }
            onClick={async () => {
              if (!plan || !target || !reservation) return;
              await reschedule.mutateAsync({
                uuid: reservation.uuid,
                body: {
                  starts_at: plan.startsAt,
                  lines: plan.lines,
                  expected_version: reservation.version,
                },
              });
              onOpenChange(false);
            }}
          >
            {t("reservations.actions.reschedule")}
          </Button>
        </>
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {target && reservation && plan ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt className="text-muted-foreground">
            {t("reservations.fields.customer")}
          </dt>
          <dd>{reservation.customer.name}</dd>
          <dt className="text-muted-foreground">
            {t("reservations.reschedule.current")}
          </dt>
          <dd className="tabular-nums">
            {datetimeInZone(reservation.starts_at, zone, locale, {
              dateStyle: "medium",
              timeStyle: "short",
            })}
          </dd>
          <dt className="text-muted-foreground">
            {t("reservations.calendar.new_time")}
          </dt>
          <dd className="font-medium tabular-nums">
            {datetimeInZone(plan.startsAt, zone, locale, {
              dateStyle: "medium",
              timeStyle: "short",
            })}
          </dd>
          {plan.staff ? (
            <>
              <dt className="text-muted-foreground">
                {t("reservations.fields.staff")}
              </dt>
              <dd>
                {target.item.service_name}: {plan.staff.name}
              </dd>
            </>
          ) : null}
          <dt className="text-muted-foreground">
            {t("reservations.calendar.line")}
          </dt>
          <dd className="tabular-nums">
            {clock(target.point.minutes)} · {target.item.service_name}
          </dd>
        </dl>
      ) : null}
    </AppDialog>
  );
}

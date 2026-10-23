"use client";

import {
  Ban,
  CalendarClock,
  Check,
  CheckCheck,
  Eye,
  Play,
  UserX,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import {
  ACTION_LABEL_KEYS,
  allowedActions,
  canReschedule,
} from "@/features/reservations/constants";
import { useCanWriteReservations } from "@/features/reservations/lib/permissions";
import type {
  ReservationAction,
  ReservationListItem,
} from "@/features/reservations/types";
import { useLocale } from "@/providers/locale-provider";

export const ACTION_ICONS: Record<ReservationAction, LucideIcon> = {
  accept: Check,
  reject: X,
  cancel: Ban,
  start: Play,
  complete: CheckCheck,
  "no-show": UserX,
};

export type ReservationRowActionHandlers = {
  onView: (row: ReservationListItem) => void;
  onAction: (row: ReservationListItem, action: ReservationAction) => void;
  onReschedule: (row: ReservationListItem) => void;
};

export function ReservationRowActions({
  reservation,
  handlers,
}: {
  reservation: ReservationListItem;
  handlers: ReservationRowActionHandlers;
}) {
  const { t } = useLocale();
  const canWrite = useCanWriteReservations();
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("reservations.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(reservation),
    },
  ];
  if (canWrite) {
    if (canReschedule(reservation.status)) {
      actions.push({
        id: "reschedule",
        label: t("reservations.actions.reschedule"),
        icon: CalendarClock,
        onSelect: () => handlers.onReschedule(reservation),
      });
    }
    for (const action of allowedActions(
      reservation.status,
      reservation.approval_status,
    )) {
      actions.push({
        id: action,
        label: t(ACTION_LABEL_KEYS[action]),
        icon: ACTION_ICONS[action],
        variant:
          action === "cancel" || action === "reject" || action === "no-show"
            ? "destructive"
            : "default",
        separatorBefore: action === "cancel",
        onSelect: () => handlers.onAction(reservation, action),
      });
    }
  }
  return <EntityRowActions actions={actions} />;
}

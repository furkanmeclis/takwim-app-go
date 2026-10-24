"use client";

import { MessageSquareWarning, PhoneOff } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { channelLabel } from "@/features/customer-notifications/constants";
import type {
  Reservation,
  ReservationConfirmation,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Tone = "default" | "success" | "warning" | "danger";

const TASK_TONES: Record<ReservationConfirmation["status"], Tone> = {
  scheduled: "default",
  pending: "warning",
  confirmed: "success",
  declined: "danger",
  expired: "danger",
  dropped: "default",
};

/**
 * The confirmation request of a booking (TEC-921): planned and sent
 * requests, the deadline, the customer's answer and what happened without
 * one. A change request or a flagged booking is called out on top, since
 * the business has to act on it.
 */
export function ReservationConfirmationCard({
  reservation,
}: {
  reservation: Reservation;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  const c = reservation.confirmation;
  if (!c) return null;

  const when = (iso: string | null | undefined) =>
    iso
      ? datetimeInZone(iso, zone, locale, {
          dateStyle: "medium",
          timeStyle: "short",
        })
      : "—";
  const open =
    reservation.status === "active" &&
    (c.status === "scheduled" ||
      c.status === "pending" ||
      c.status === "expired");
  const answer = c.answer
    ? [
        t(`reservations.confirmation_task.answers.${c.answer}`),
        c.answer_channel
          ? t(`reservations.confirmation_task.sources.${c.answer_channel}`)
          : "",
        when(c.answered_at),
      ]
        .filter(Boolean)
        .join(" · ")
    : "—";

  return (
    <div className="space-y-3">
      {open && c.change_requested_at ? (
        <Alert>
          <MessageSquareWarning className="size-4" />
          <AlertTitle>
            {t("reservations.confirmation_task.change_requested_title", {
              at: when(c.change_requested_at),
            })}
          </AlertTitle>
          <AlertDescription>
            {c.change_note ||
              t("reservations.confirmation_task.change_requested_empty")}
          </AlertDescription>
        </Alert>
      ) : null}
      {open && c.outcome && c.outcome !== "cancelled" ? (
        <Alert variant="destructive">
          <PhoneOff className="size-4" />
          <AlertTitle>
            {t(`reservations.confirmation_task.outcome_alert.${c.outcome}`)}
          </AlertTitle>
          <AlertDescription>
            {t("reservations.confirmation_task.outcome_alert.description")}
          </AlertDescription>
        </Alert>
      ) : null}
      <EntitySectionCard
        title={t("reservations.confirmation_task.title")}
        actions={
          <StatusChip
            label={t(`reservations.confirmation_task.status.${c.status}`)}
            tone={TASK_TONES[c.status]}
          />
        }
      >
        <EntityDetail
          sections={[
            {
              id: "confirmation",
              fields: [
                {
                  key: "channels",
                  label: t("reservations.confirmation_task.channels"),
                  value:
                    c.channels.map((ch) => channelLabel(t, ch)).join(" → ") ||
                    "—",
                },
                {
                  key: "attempts",
                  label: t("reservations.confirmation_task.attempts"),
                  value: t("reservations.confirmation_task.attempts_value", {
                    sent: c.attempts_sent,
                    planned: c.attempts_planned,
                  }),
                },
                {
                  key: "next_attempt",
                  label: t("reservations.confirmation_task.next_attempt"),
                  value: when(c.next_attempt_at),
                },
                {
                  key: "deadline",
                  label: t("reservations.confirmation_task.deadline"),
                  value: when(c.deadline_at),
                },
                {
                  key: "on_no_answer",
                  label: t("reservations.confirmation_task.on_no_answer"),
                  value: t(
                    `reservations.confirmation_task.on_no_answer_values.${c.on_no_answer}`,
                  ),
                },
                {
                  key: "answer",
                  label: t("reservations.confirmation_task.answer"),
                  value: answer,
                },
                ...(c.change_requested_at
                  ? [
                      {
                        key: "change",
                        label: t("reservations.confirmation_task.change"),
                        value: [when(c.change_requested_at), c.change_note]
                          .filter(Boolean)
                          .join(" · "),
                      },
                    ]
                  : []),
                ...(c.outcome
                  ? [
                      {
                        key: "outcome",
                        label: t("reservations.confirmation_task.outcome"),
                        value: t(
                          `reservations.confirmation_task.outcomes.${c.outcome}`,
                        ),
                      },
                    ]
                  : []),
                ...(c.last_error
                  ? [
                      {
                        key: "last_error",
                        label: t("reservations.confirmation_task.last_error"),
                        value: c.last_error,
                      },
                    ]
                  : []),
              ],
            },
          ]}
        />
      </EntitySectionCard>
    </div>
  );
}

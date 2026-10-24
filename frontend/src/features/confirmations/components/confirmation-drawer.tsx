"use client";

import {
  Ban,
  CalendarClock,
  ExternalLink,
  MessageSquareWarning,
  Phone,
  PhoneOff,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { EntityDetail, EntityDrawer } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { ConfirmationAttemptsTable } from "@/features/confirmations/components/confirmation-attempts-table";
import {
  ANSWER_KEYS,
  ANSWER_SOURCES,
  SEND_STATUS,
  STATES,
  isActiveVisit,
  telHref,
} from "@/features/confirmations/lib/list";
import type { ConfirmationTask } from "@/features/confirmations/types";
import { channelLabel } from "@/features/customer-notifications/constants";
import {
  ApprovalChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-status-chips";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type ConfirmationQuickActions = {
  canWrite: boolean;
  onConfirm: (row: ConfirmationTask) => void;
  onReschedule: (row: ConfirmationTask) => void;
  onCancel: (row: ConfirmationTask) => void;
};

/**
 * One booking of the confirmation list: the business's approval and the
 * customer's confirmation, the requests on every channel, the answer, and
 * the quick actions of the desk (call, confirmed on the phone, move,
 * cancel, open the visit).
 */
export function ConfirmationDrawer({
  row,
  onOpenChange,
  actions,
}: {
  row: ConfirmationTask | null;
  onOpenChange: (open: boolean) => void;
  actions: ConfirmationQuickActions;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  const when = (iso: string | null | undefined) =>
    iso
      ? datetimeInZone(iso, zone, locale, {
          dateStyle: "medium",
          timeStyle: "short",
        })
      : "—";

  const tel = telHref(row?.customer.phone);
  const active = row ? isActiveVisit(row) : false;

  return (
    <EntityDrawer
      open={Boolean(row)}
      onOpenChange={onOpenChange}
      size="xl"
      title={
        row
          ? t("confirmations.drawer.title", {
              number: row.number,
              customer: row.customer.name,
            })
          : ""
      }
      description={
        row
          ? [when(row.starts_at), row.branch.name, row.services]
              .filter(Boolean)
              .join(" · ")
          : undefined
      }
      footer={
        row ? (
          <div className="flex flex-wrap justify-end gap-2">
            {tel ? (
              <Button asChild variant="outline" size="sm">
                <a href={tel}>
                  <Phone className="size-4" />
                  {t("confirmations.actions.call")}
                </a>
              </Button>
            ) : null}
            <Button asChild variant="outline" size="sm">
              <Link href={routes.tenant.reservations.detail(slug, row.uuid)}>
                <ExternalLink className="size-4" />
                {t("confirmations.actions.open")}
              </Link>
            </Button>
            {actions.canWrite && active ? (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => actions.onReschedule(row)}
                >
                  <CalendarClock className="size-4" />
                  {t("confirmations.actions.reschedule")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => actions.onCancel(row)}
                >
                  <Ban className="size-4" />
                  {t("confirmations.actions.cancel")}
                </Button>
              </>
            ) : null}
            {actions.canWrite && row.can_confirm ? (
              <Button size="sm" onClick={() => actions.onConfirm(row)}>
                <ShieldCheck className="size-4" />
                {t("confirmations.phone.action")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {row ? (
        <div className="space-y-4">
          {row.state === "change_requested" ? (
            <Alert>
              <MessageSquareWarning className="size-4" />
              <AlertTitle>
                {t("reservations.confirmation_task.change_requested_title", {
                  at: when(row.change_requested_at),
                })}
              </AlertTitle>
              <AlertDescription>
                {row.change_note ||
                  t("reservations.confirmation_task.change_requested_empty")}
              </AlertDescription>
            </Alert>
          ) : null}
          {row.attention &&
          (row.state === "unreachable" || row.state === "flagged") ? (
            <Alert variant="destructive">
              <PhoneOff className="size-4" />
              <AlertTitle>
                {t(`reservations.confirmation_task.outcome_alert.${row.state}`)}
              </AlertTitle>
              <AlertDescription>
                {t("reservations.confirmation_task.outcome_alert.description")}
              </AlertDescription>
            </Alert>
          ) : null}
          {row.attention && row.send_failed ? (
            <Alert variant="destructive">
              <PhoneOff className="size-4" />
              <AlertTitle>{t("confirmations.drawer.send_failed")}</AlertTitle>
              <AlertDescription>
                {row.last_send?.error ||
                  t("confirmations.drawer.send_failed_hint")}
              </AlertDescription>
            </Alert>
          ) : null}
          <EntityDetail
            sections={[
              {
                id: "confirmation",
                fields: [
                  {
                    key: "visit",
                    label: t("confirmations.list.columns.status"),
                    value: <ReservationStatusChip status={row.status} />,
                  },
                  {
                    key: "approval",
                    label: t("confirmations.list.columns.approval_status"),
                    value: <ApprovalChip status={row.approval_status} />,
                  },
                  {
                    key: "state",
                    label: t("confirmations.list.columns.state"),
                    value: (
                      <StatusChip
                        label={t(STATES[row.state].key)}
                        tone={STATES[row.state].tone}
                      />
                    ),
                  },
                  {
                    key: "channels",
                    label: t("reservations.confirmation_task.channels"),
                    value:
                      row.channels
                        .map((ch) => channelLabel(t, ch))
                        .join(" → ") || "—",
                  },
                  {
                    key: "attempts",
                    label: t("reservations.confirmation_task.attempts"),
                    value: t("reservations.confirmation_task.attempts_value", {
                      sent: row.attempts_sent,
                      planned: row.attempts_planned,
                    }),
                  },
                  {
                    key: "last_send",
                    label: t("confirmations.list.columns.last_send"),
                    value: row.last_send ? (
                      <StatusChip
                        label={t(
                          SEND_STATUS[row.last_send.status]?.key ??
                            row.last_send.status,
                        )}
                        tone={SEND_STATUS[row.last_send.status]?.tone}
                      />
                    ) : (
                      "—"
                    ),
                  },
                  {
                    key: "next_attempt",
                    label: t("reservations.confirmation_task.next_attempt"),
                    value: when(row.next_attempt_at),
                  },
                  {
                    key: "deadline",
                    label: t("reservations.confirmation_task.deadline"),
                    value: when(row.deadline_at),
                  },
                  {
                    key: "on_no_answer",
                    label: t("reservations.confirmation_task.on_no_answer"),
                    value: t(
                      `reservations.confirmation_task.on_no_answer_values.${row.on_no_answer}`,
                    ),
                  },
                  {
                    key: "answer",
                    label: t("reservations.confirmation_task.answer"),
                    value: row.answer
                      ? [
                          t(ANSWER_KEYS[row.answer] ?? row.answer),
                          row.answer_channel
                            ? t(
                                ANSWER_SOURCES[row.answer_channel] ??
                                  row.answer_channel,
                              )
                            : "",
                          when(row.answered_at),
                        ]
                          .filter(Boolean)
                          .join(" · ")
                      : "—",
                  },
                  ...(row.last_error
                    ? [
                        {
                          key: "last_error",
                          label: t("reservations.confirmation_task.last_error"),
                          value: row.last_error,
                        },
                      ]
                    : []),
                ],
              },
            ]}
          />
          <div className="space-y-2">
            <h3 className="text-sm font-medium">
              {t("confirmations.attempts.title")}
            </h3>
            <ConfirmationAttemptsTable reservationUuid={row.uuid} />
          </div>
        </div>
      ) : null}
    </EntityDrawer>
  );
}

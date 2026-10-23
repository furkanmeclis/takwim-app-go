"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  APPROVAL,
  CONFIRMATION,
  PAYMENT,
  STATUS,
} from "@/features/reservations/constants";
import type {
  ApprovalStatus,
  ConfirmationStatus,
  PaymentStatus,
  ReservationStatus,
} from "@/features/reservations/types";
import { useLocale } from "@/providers/locale-provider";

export function ReservationStatusChip({
  status,
}: {
  status: ReservationStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip label={t(STATUS[status].key)} tone={STATUS[status].tone} />
  );
}

export function ApprovalChip({ status }: { status: ApprovalStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip label={t(APPROVAL[status].key)} tone={APPROVAL[status].tone} />
  );
}

export function ConfirmationChip({ status }: { status: ConfirmationStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(CONFIRMATION[status].key)}
      tone={CONFIRMATION[status].tone}
    />
  );
}

export function PaymentChip({ status }: { status: PaymentStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip label={t(PAYMENT[status].key)} tone={PAYMENT[status].tone} />
  );
}

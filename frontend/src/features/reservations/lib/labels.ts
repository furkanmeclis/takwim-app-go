import {
  APPROVAL,
  CANCEL_REASON_KEYS,
  CONFIRMATION,
  PAYMENT,
  SOURCE,
  STATUS,
} from "@/features/reservations/constants";

type Translate = (key: string) => string;

export const statusLabel = (t: Translate, v: string) =>
  STATUS[v as keyof typeof STATUS]
    ? t(STATUS[v as keyof typeof STATUS].key)
    : v;

export const approvalLabel = (t: Translate, v: string) =>
  APPROVAL[v as keyof typeof APPROVAL]
    ? t(APPROVAL[v as keyof typeof APPROVAL].key)
    : v;

export const confirmationLabel = (t: Translate, v: string) =>
  CONFIRMATION[v as keyof typeof CONFIRMATION]
    ? t(CONFIRMATION[v as keyof typeof CONFIRMATION].key)
    : v;

export const paymentLabel = (t: Translate, v: string) =>
  PAYMENT[v as keyof typeof PAYMENT]
    ? t(PAYMENT[v as keyof typeof PAYMENT].key)
    : v;

export const sourceLabel = (t: Translate, v: string) =>
  SOURCE[v as keyof typeof SOURCE] ? t(SOURCE[v as keyof typeof SOURCE]) : v;

export const cancelReasonLabel = (t: Translate, v: string) =>
  CANCEL_REASON_KEYS[v] ? t(CANCEL_REASON_KEYS[v]) : v;

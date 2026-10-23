import type {
  ApprovalStatus,
  ConfirmationStatus,
  PaymentStatus,
  ReservationAction,
  ReservationSource,
  ReservationStatus,
} from "@/features/reservations/types";

/** Resource slug shared by activity and live lists. */
export const RESERVATIONS_RESOURCE = "tenant.reservations" as const;

export type Tone = "default" | "success" | "warning" | "danger";

/** Static label keys (no template keys) and chip tones of every state. */
export const STATUS: Record<ReservationStatus, { key: string; tone: Tone }> = {
  requested: { key: "reservations.status.requested", tone: "warning" },
  active: { key: "reservations.status.active", tone: "success" },
  in_service: { key: "reservations.status.in_service", tone: "success" },
  completed: { key: "reservations.status.completed", tone: "default" },
  cancelled: { key: "reservations.status.cancelled", tone: "danger" },
  rejected: { key: "reservations.status.rejected", tone: "danger" },
  no_show: { key: "reservations.status.no_show", tone: "danger" },
};

export const APPROVAL: Record<ApprovalStatus, { key: string; tone: Tone }> = {
  pending: { key: "reservations.approval.pending", tone: "warning" },
  accepted: { key: "reservations.approval.accepted", tone: "success" },
  rejected: { key: "reservations.approval.rejected", tone: "danger" },
};

export const CONFIRMATION: Record<
  ConfirmationStatus,
  { key: string; tone: Tone }
> = {
  not_required: {
    key: "reservations.confirmation.not_required",
    tone: "default",
  },
  scheduled: { key: "reservations.confirmation.scheduled", tone: "default" },
  pending: { key: "reservations.confirmation.pending", tone: "warning" },
  confirmed: { key: "reservations.confirmation.confirmed", tone: "success" },
  declined: { key: "reservations.confirmation.declined", tone: "danger" },
  expired: { key: "reservations.confirmation.expired", tone: "danger" },
};

export const PAYMENT: Record<PaymentStatus, { key: string; tone: Tone }> = {
  on_site: { key: "reservations.payment.on_site", tone: "default" },
  online_pending: {
    key: "reservations.payment.online_pending",
    tone: "warning",
  },
  paid: { key: "reservations.payment.paid", tone: "success" },
  refund_pending: {
    key: "reservations.payment.refund_pending",
    tone: "warning",
  },
  partially_refunded: {
    key: "reservations.payment.partially_refunded",
    tone: "default",
  },
  refunded: { key: "reservations.payment.refunded", tone: "default" },
  failed: { key: "reservations.payment.failed", tone: "danger" },
};

export const SOURCE: Record<ReservationSource, string> = {
  panel: "reservations.source.panel",
  online: "reservations.source.online",
  embed: "reservations.source.embed",
};

/** History entry types (`events[].type`). */
export const EVENT_LABEL_KEYS: Record<string, string> = {
  created: "reservations.events.created",
  accepted: "reservations.events.accepted",
  rejected: "reservations.events.rejected",
  cancelled: "reservations.events.cancelled",
  started: "reservations.events.started",
  completed: "reservations.events.completed",
  no_show: "reservations.events.no_show",
  paid: "reservations.events.paid",
  expired: "reservations.events.expired",
  rescheduled: "reservations.events.rescheduled",
  notes_updated: "reservations.events.notes_updated",
};

/** System cancel reasons; people's own reasons are shown as typed. */
export const CANCEL_REASON_KEYS: Record<string, string> = {
  cancelled: "reservations.cancel_reasons.cancelled",
  customer_request: "reservations.cancel_reasons.customer_request",
  business: "reservations.cancel_reasons.business",
  approval_timeout: "reservations.cancel_reasons.approval_timeout",
  payment_timeout: "reservations.cancel_reasons.payment_timeout",
  customer_cancelled: "reservations.cancel_reasons.customer_cancelled",
};

/** Which actions a state allows (mirrors the API's transitions). */
export function allowedActions(
  status: ReservationStatus,
  approval: ApprovalStatus,
): ReservationAction[] {
  switch (status) {
    case "requested":
      return approval === "pending"
        ? ["accept", "reject", "cancel"]
        : ["cancel"];
    case "active":
      return ["start", "complete", "no-show", "cancel"];
    case "in_service":
      return ["complete"];
    default:
      return [];
  }
}

export const ACTION_LABEL_KEYS: Record<ReservationAction, string> = {
  accept: "reservations.actions.accept",
  reject: "reservations.actions.reject",
  cancel: "reservations.actions.cancel",
  start: "reservations.actions.start",
  complete: "reservations.actions.complete",
  "no-show": "reservations.actions.no_show",
};

/** A request or an active booking can move. */
export function canReschedule(status: ReservationStatus) {
  return status === "requested" || status === "active";
}

export const MAX_LINES = 10;
export const MAX_NOTE = 2000;
/** Guests of one booking (restaurant flow), as the API allows. */
export const MIN_PARTY_SIZE = 1;
export const MAX_PARTY_SIZE = 1000;

/** Vehicle types of a car wash booking (API enum) and their label keys. */
export const VEHICLE_TYPES = [
  "car",
  "suv",
  "van",
  "motorcycle",
  "other",
] as const;
export type VehicleType = (typeof VEHICLE_TYPES)[number];
export const VEHICLE_TYPE_KEYS: Record<VehicleType, string> = {
  car: "reservations.vehicle.car",
  suv: "reservations.vehicle.suv",
  van: "reservations.vehicle.van",
  motorcycle: "reservations.vehicle.motorcycle",
  other: "reservations.vehicle.other",
};
export const MAX_PLATE = 16;

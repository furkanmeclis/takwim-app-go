import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Reservation = Schemas["TenantReservation"];
export type ReservationListItem = Schemas["TenantReservationListItem"];
export type ReservationPage = Schemas["TenantReservationPage"];
export type ReservationFacets = Schemas["TenantReservationFacets"];
/** Booking form questions of the business (TEC-905). */
export type ReservationForm = Schemas["EnvelopeTenantReservationForm"]["data"];
export type ReservationFormField = ReservationForm["fields"][number];
export type ReservationAnswer = Schemas["TenantReservationAnswer"];
/** Money of one visit (`GET /v1/tenant/reservations/{uuid}/payments`). */
export type ReservationPayments = Schemas["ReservationPayments"];
export type ReservationLedgerEntry = Schemas["LedgerEntry"];
export type PaymentAttempt = Schemas["PaymentAttempt"];
export type CollectAccount = Schemas["CollectAccount"];
export type ReservationCollectionRequest =
  Schemas["ReservationCollectionRequest"];
export type CollectMethod = ReservationCollectionRequest["method"];
export type Refund = Schemas["Refund"];
export type RefundCollection = Schemas["RefundCollection"];
export type ReservationRefundRequest = Schemas["ReservationRefundRequest"];
export type RefundResult = Schemas["RefundResult"];
export type RefundMethod = NonNullable<ReservationRefundRequest["method"]>;
/** Closes a refund to make without paying it back (TEC-978). */
export type ReservationCloseWithoutRefundRequest =
  Schemas["ReservationCloseWithoutRefundRequest"];
export type RefundResolveRequest = {
  outcome: "succeeded" | "failed";
  note?: string;
};
/** Visits whose money needs a person (`GET /v1/tenant/payments/follow-ups`). */
export type PaymentFollowUp = Schemas["PaymentFollowUp"];
export type PaymentFollowUpPage = Schemas["PaymentFollowUpPage"];
export type PaymentFollowUpKind = PaymentFollowUp["kind"];
export type PaymentFollowUpFacets =
  Schemas["EnvelopePaymentFollowUpFacets"]["data"];
export type ListPaymentFollowUpsParams = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  kind?: string;
  branch?: string;
};
export type ReservationCreateRequest =
  Schemas["TenantReservationCreateRequest"];
export type ReservationRescheduleRequest =
  Schemas["TenantReservationRescheduleRequest"];
export type ReservationNotesRequest = Schemas["TenantReservationNotesRequest"];
export type ReservationActionRequest =
  Schemas["TenantReservationActionRequest"];
export type ReservationLineInput = Schemas["TenantReservationLineInput"];
export type ReservationStatus = Schemas["TenantReservationStatus"];
export type ApprovalStatus = Schemas["TenantReservationApprovalStatus"];
export type ConfirmationStatus = Schemas["TenantReservationConfirmationStatus"];
/** The confirmation task of a booking (TEC-921). */
export type ReservationConfirmation = Schemas["TenantReservationConfirmation"];
export type PaymentStatus = Schemas["TenantReservationPaymentStatus"];
export type ReservationSource = Schemas["TenantReservationSource"];
export type ReservationEvent = Reservation["events"][number];
export type ReservationLine = Reservation["lines"][number];
/** A product taken with the visit (TEC-919). */
export type ReservationProduct = Schemas["TenantReservationProduct"];
export type ReservationProductRequest =
  Schemas["TenantReservationProductRequest"];
export type ReservationProductOption =
  Schemas["TenantReservationProductOption"];
/** A session of the customer's package on a line (TEC-920). */
export type ReservationPackageSession =
  Schemas["TenantReservationPackageSession"];
export type ReservationPackageSessionRequest =
  Schemas["TenantReservationPackageSessionRequest"];
export type ReservationPackageOption =
  Schemas["TenantReservationPackageOption"];

export type ReservationCalendar = Schemas["TenantReservationCalendar"];
export type CalendarColumn = Schemas["TenantReservationCalendarColumn"];
export type CalendarItem = Schemas["TenantReservationCalendarItem"];
export type CalendarInterval = Schemas["TenantReservationInterval"];
export type CalendarBy = ReservationCalendar["by"];

/** Query of `GET /v1/tenant/reservations/calendar`. */
export type CalendarParams = {
  from: string;
  to: string;
  by: CalendarBy;
  branch?: string;
  include_cancelled?: "true";
};

export type ReservationAgenda = Schemas["TenantReservationAgenda"];
export type AgendaItem = Schemas["TenantReservationAgendaItem"];
export type AgendaWork = Schemas["TenantReservationAgendaWork"];

/** Query of `GET /v1/tenant/reservations/agenda`. */
export type AgendaParams = {
  from: string;
  to: string;
  include_cancelled?: "true";
};

export type AvailabilitySearchRequest =
  Schemas["TenantAvailabilitySearchRequest"];
export type Availability = Schemas["TenantAvailability"];
export type AvailabilitySlot = Schemas["TenantAvailabilitySlot"];

/** Status changes of `POST /v1/tenant/reservations/{uuid}/{action}`. */
export type ReservationAction =
  "accept" | "reject" | "cancel" | "start" | "complete" | "no-show";

/** List query of `GET /v1/tenant/reservations`. */
export type ListReservationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** Comma-separated values (any of). */
  branch?: string;
  staff?: string;
  status?: string;
  approval_status?: string;
  confirmation_status?: string;
  payment_status?: string;
  source?: string;
  customer_uuid?: string;
  starts_from?: string;
  starts_to?: string;
};

import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Reservation = Schemas["TenantReservation"];
export type ReservationListItem = Schemas["TenantReservationListItem"];
export type ReservationPage = Schemas["TenantReservationPage"];
export type ReservationFacets = Schemas["TenantReservationFacets"];
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
export type PaymentStatus = Schemas["TenantReservationPaymentStatus"];
export type ReservationSource = Schemas["TenantReservationSource"];
export type ReservationEvent = Reservation["events"][number];
export type ReservationLine = Reservation["lines"][number];

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
  payment_status?: string;
  source?: string;
  customer_uuid?: string;
  starts_from?: string;
  starts_to?: string;
};

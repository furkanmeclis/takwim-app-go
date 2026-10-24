import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** Public booking API (`/v1/public/businesses/{slug}`, TEC-906). */
export type PublicBusiness = Schemas["PublicBusiness"];
export type PublicBranch = Schemas["PublicBranch"];
export type PublicService = Schemas["PublicService"];
export type PublicStaff = Schemas["PublicStaff"];
export type PublicLineInput = Schemas["PublicLineInput"];
export type PublicAvailabilityRequest = Schemas["PublicAvailabilityRequest"];
export type PublicAvailability = Schemas["PublicAvailability"];
export type PublicDay = PublicAvailability["days"][number];
export type PublicSlot = PublicDay["slots"][number];
/** Class timetable of a branch (TEC-990). */
export type PublicClassTimetable = Schemas["PublicClassTimetable"];
export type PublicClassSession = Schemas["PublicClassSession"];
export type PublicClassesQuery = {
  branch_uuid: string;
  service_uuid?: string;
  from: string;
  to: string;
  party_size: number;
  channel: "web" | "widget";
};
export type PublicCheckoutRequest = Schemas["PublicCheckoutRequest"];
export type PublicCheckout = Schemas["PublicCheckout"];
export type PublicBookingRequest = Schemas["PublicBookingRequest"];
export type PublicReservation = Schemas["PublicReservation"];
/** A product sold online in a branch (TEC-919). */
export type PublicProduct = Schemas["PublicProduct"];
export type PublicBookingProduct = NonNullable<
  PublicBookingRequest["products"]
>[number];
/** Vehicle of a car wash booking (TEC-904 / TEC-971). */
export type PublicVehicle = Schemas["ReservationVehicle"];
export type OnlinePaymentStart = Schemas["OnlinePaymentStart"];
export type TestPaymentPage = Schemas["TestPaymentPage"];
export type PaymentNotifyResult = Schemas["PaymentNotifyResult"];
export type PaymentMethod =
  PublicBusiness["booking"]["payment_methods"][number];

/** Wizard steps the business page shows (subset of the sector flow). */
export type WizardStep =
  | "branch"
  | "vehicle"
  | "party_size"
  | "area"
  | "service"
  | "staff"
  | "datetime"
  | "products"
  | "contact"
  | "summary";

/** Contact form of the booking (phone OTP comes with TEC-909 / TEC-910). */
export type ContactValues = {
  first_name: string;
  last_name: string;
  phone: string;
  email: string;
  customer_note: string;
  sms_consent: boolean;
  email_consent: boolean;
  /** Answers of the business's online questions (TEC-905). */
  custom_fields: Record<string, string | boolean>;
  /** Honeypot: hidden from people, must stay empty. */
  website: string;
};

/** A booking question asked online (`booking.fields`). */
export type PublicBookingField = PublicBusiness["booking"]["fields"][number];

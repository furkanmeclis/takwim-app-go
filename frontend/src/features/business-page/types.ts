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
export type PublicCheckoutRequest = Schemas["PublicCheckoutRequest"];
export type PublicCheckout = Schemas["PublicCheckout"];
export type PublicBookingRequest = Schemas["PublicBookingRequest"];
export type PublicReservation = Schemas["PublicReservation"];
export type PaymentMethod =
  PublicBusiness["booking"]["payment_methods"][number];

/** Wizard steps the business page shows (subset of the sector flow). */
export type WizardStep =
  "branch" | "service" | "staff" | "datetime" | "contact" | "summary";

/** Contact form of the booking (phone OTP comes with TEC-909 / TEC-910). */
export type ContactValues = {
  first_name: string;
  last_name: string;
  phone: string;
  email: string;
  customer_note: string;
  sms_consent: boolean;
  email_consent: boolean;
  /** Honeypot: hidden from people, must stay empty. */
  website: string;
};

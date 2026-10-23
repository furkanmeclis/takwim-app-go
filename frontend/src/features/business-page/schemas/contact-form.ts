import { z } from "zod";

import type {
  ContactValues,
  PaymentMethod,
  PublicBookingRequest,
} from "@/features/business-page/types";

type Translate = (key: string) => string;

const MAX_NAME = 80;
const MAX_EMAIL = 254;
const MAX_NOTE = 2000;

/** Contact form of the booking; the API normalizes the phone to E.164. */
export function createContactSchema(t: Translate) {
  return z.object({
    first_name: z
      .string()
      .trim()
      .min(1, t("business_page.validation.first_name_required"))
      .max(MAX_NAME, t("business_page.validation.too_long")),
    last_name: z
      .string()
      .trim()
      .max(MAX_NAME, t("business_page.validation.too_long")),
    phone: z
      .string()
      .trim()
      .refine((v) => {
        const digits = v.replace(/\D/g, "").length;
        return digits >= 10 && digits <= 15;
      }, t("business_page.validation.phone_invalid")),
    email: z
      .string()
      .trim()
      .max(MAX_EMAIL, t("business_page.validation.too_long"))
      .refine(
        (v) => !v || z.email().safeParse(v).success,
        t("business_page.validation.email_invalid"),
      ),
    customer_note: z
      .string()
      .trim()
      .max(MAX_NOTE, t("business_page.validation.too_long")),
    sms_consent: z.boolean(),
    email_consent: z.boolean(),
    website: z.string(),
  }) satisfies z.ZodType<ContactValues>;
}

export function contactDefaults(): ContactValues {
  return {
    first_name: "",
    last_name: "",
    phone: "",
    email: "",
    customer_note: "",
    sms_consent: false,
    email_consent: false,
    website: "",
  };
}

/** Booking body of a held checkout and the contact form. */
export function toBookingRequest(
  checkoutToken: string,
  values: ContactValues,
  payment: PaymentMethod | undefined,
): PublicBookingRequest {
  return {
    checkout_token: checkoutToken,
    customer: {
      first_name: values.first_name,
      last_name: values.last_name,
      phone: values.phone,
      email: values.email,
      sms_consent: values.sms_consent,
      email_consent: values.email_consent,
    },
    ...(payment ? { payment } : {}),
    customer_note: values.customer_note,
    website: values.website,
  };
}

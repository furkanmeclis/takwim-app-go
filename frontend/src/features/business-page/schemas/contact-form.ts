import { z } from "zod";

import type {
  ContactValues,
  PaymentMethod,
  PublicBookingField,
  PublicBookingProduct,
  PublicBookingRequest,
} from "@/features/business-page/types";
import {
  customDefaults,
  customFieldIssues,
  toCustomFields,
} from "@/features/reservations/lib/custom-fields";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const MAX_NAME = 80;
const MAX_EMAIL = 254;
const MAX_NOTE = 2000;

/**
 * Contact form of the booking; the API normalizes the phone to E.164 and
 * checks the answers of the business's questions again.
 */
export function createContactSchema(
  t: Translate,
  fields: PublicBookingField[] = [],
) {
  return z
    .object({
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
      custom_fields: z.record(z.string(), z.union([z.string(), z.boolean()])),
      website: z.string(),
    })
    .superRefine((values, ctx) => {
      for (const issue of customFieldIssues(fields, values.custom_fields, t)) {
        ctx.addIssue({
          code: "custom",
          path: ["custom_fields", issue.key],
          message: issue.message,
        });
      }
    }) satisfies z.ZodType<ContactValues>;
}

export function contactDefaults(
  fields: PublicBookingField[] = [],
): ContactValues {
  return {
    custom_fields: customDefaults(fields),
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
  products: PublicBookingProduct[] = [],
  fields: PublicBookingField[] = [],
): PublicBookingRequest {
  const answers = toCustomFields(fields, values.custom_fields);
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
    ...(Object.keys(answers).length > 0 ? { custom_fields: answers } : {}),
    website: values.website,
    ...(products.length > 0 ? { products } : {}),
  };
}

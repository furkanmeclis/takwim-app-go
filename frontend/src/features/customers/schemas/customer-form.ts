import { z } from "zod";

import {
  MAX_NAME,
  MAX_NOTES,
  MAX_TAG_LENGTH,
  MAX_TAGS,
} from "@/features/customers/constants";
import type {
  Customer,
  CustomerCreateRequest,
} from "@/features/customers/types";

type Translate = (key: string) => string;

/** "vip, düzenli" → ["vip", "düzenli"]; the API trims and deduplicates too. */
export function splitTags(raw: string): string[] {
  return raw
    .split(",")
    .map((tag) => tag.trim().replace(/\s+/g, " "))
    .filter(Boolean);
}

function phoneDigits(raw: string) {
  return raw.replace(/\D/g, "");
}

export function createCustomerFormSchema(t: Translate) {
  return z.object({
    first_name: z
      .string()
      .trim()
      .min(1, t("customers.validation.first_name_required"))
      .max(MAX_NAME, t("customers.validation.name_max")),
    last_name: z
      .string()
      .trim()
      .max(MAX_NAME, t("customers.validation.name_max")),
    // The API normalizes to E.164; here only an obviously short or long
    // number is stopped.
    phone: z
      .string()
      .trim()
      .refine((v) => {
        if (!v) return true;
        const digits = phoneDigits(v).length;
        return digits >= 7 && digits <= 15;
      }, t("customers.validation.phone_invalid")),
    email: z
      .string()
      .trim()
      .refine(
        (v) => !v || z.email().safeParse(v).success,
        t("customers.validation.email_invalid"),
      ),
    birth_date: z.string(),
    tags: z.string().refine((v) => {
      const tags = splitTags(v);
      return (
        tags.length <= MAX_TAGS &&
        tags.every((tag) => tag.length <= MAX_TAG_LENGTH)
      );
    }, t("customers.validation.tags")),
    notes: z.string().max(MAX_NOTES, t("customers.validation.notes_max")),
    sms_consent: z.boolean(),
    email_consent: z.boolean(),
  });
}

export type CustomerFormValues = z.infer<
  ReturnType<typeof createCustomerFormSchema>
>;

export function customerFormDefaults(customer?: Customer): CustomerFormValues {
  return {
    first_name: customer?.first_name ?? "",
    last_name: customer?.last_name ?? "",
    phone: customer?.phone_display ?? "",
    email: customer?.email ?? "",
    birth_date: customer?.birth_date ?? "",
    tags: (customer?.tags ?? []).join(", "),
    notes: customer?.notes ?? "",
    sms_consent: customer?.sms_consent ?? false,
    email_consent: customer?.email_consent ?? false,
  };
}

export function customerFormToRequest(
  values: CustomerFormValues,
): CustomerCreateRequest {
  return {
    first_name: values.first_name.trim(),
    last_name: values.last_name.trim(),
    phone: values.phone.trim() || null,
    email: values.email.trim(),
    birth_date: values.birth_date || null,
    tags: splitTags(values.tags),
    notes: values.notes.trim(),
    sms_consent: values.sms_consent,
    email_consent: values.email_consent,
  };
}

export function createQuickEditSchema(t: Translate) {
  return createCustomerFormSchema(t).pick({
    first_name: true,
    last_name: true,
    phone: true,
    tags: true,
  });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

export function createTagSchema(t: Translate) {
  return z.object({
    tag: z
      .string()
      .trim()
      .min(1, t("customers.validation.tag_required"))
      .max(MAX_TAG_LENGTH, t("customers.validation.tags")),
  });
}

export type TagFormValues = z.infer<ReturnType<typeof createTagSchema>>;

import { z } from "zod";

import { normalizeAmount } from "@/features/cash/lib/format";
import {
  FINANCE_KINDS,
  MAX_CATEGORY_NAME,
  MAX_DESCRIPTION,
} from "@/features/finance/constants";
import type {
  FinanceEntry,
  FinanceEntryCorrectRequest,
  FinanceEntryCreateRequest,
  FinanceKind,
} from "@/features/finance/types";

type Translate = (key: string) => string;

const AMOUNT = /^\d+([.,]\d{1,2})?$/;

/** Select value of "the account's branch" / "no staff member". */
export const AUTO = "__auto";
export const NONE = "__none";

function amountField(t: Translate) {
  return z
    .string()
    .trim()
    .refine((v) => AMOUNT.test(v), t("finance.validation.amount"))
    .refine(
      (v) => Number(v.replace(",", ".")) > 0,
      t("finance.validation.amount_positive"),
    );
}

function descriptionField(t: Translate) {
  return z
    .string()
    .max(MAX_DESCRIPTION, t("finance.validation.description_max"));
}

const optionalRef = (value: string) =>
  value === AUTO || value === NONE || !value ? null : value;

// ------------------------------------------------------------------ entry

export function createEntryFormSchema(t: Translate) {
  return z.object({
    kind: z.enum(FINANCE_KINDS as [FinanceKind, ...FinanceKind[]]),
    category_uuid: z.string().min(1, t("finance.validation.category_required")),
    cash_account_uuid: z
      .string()
      .min(1, t("finance.validation.account_required")),
    branch_uuid: z.string(),
    staff_uuid: z.string(),
    amount: amountField(t),
    entry_date: z.string().min(1, t("finance.validation.date_required")),
    description: descriptionField(t),
  });
}

export type EntryFormValues = z.infer<ReturnType<typeof createEntryFormSchema>>;

export function entryFormToRequest(
  values: EntryFormValues,
  idempotencyKey: string,
): FinanceEntryCreateRequest {
  return {
    kind: values.kind,
    category_uuid: values.category_uuid,
    cash_account_uuid: values.cash_account_uuid,
    branch_uuid: optionalRef(values.branch_uuid),
    staff_uuid: optionalRef(values.staff_uuid),
    amount: normalizeAmount(values.amount),
    entry_date: values.entry_date,
    description: values.description.trim(),
    idempotency_key: idempotencyKey,
  };
}

// ------------------------------------------------------------- correction

export function createCorrectFormSchema(t: Translate) {
  return z.object({
    category_uuid: z.string().min(1, t("finance.validation.category_required")),
    cash_account_uuid: z
      .string()
      .min(1, t("finance.validation.account_required")),
    staff_uuid: z.string(),
    amount: amountField(t),
    entry_date: z.string().min(1, t("finance.validation.date_required")),
    description: descriptionField(t),
    reason: z
      .string()
      .trim()
      .min(3, t("finance.validation.reason_min"))
      .max(500, t("finance.validation.reason_max")),
  });
}

export type CorrectFormValues = z.infer<
  ReturnType<typeof createCorrectFormSchema>
>;

export function correctFormDefaults(entry: FinanceEntry): CorrectFormValues {
  return {
    category_uuid: entry.category.uuid,
    cash_account_uuid: entry.account.uuid,
    staff_uuid: entry.staff?.uuid ?? NONE,
    amount: entry.amount,
    entry_date: entry.entry_date,
    description: entry.description,
    reason: "",
  };
}

/** Only the changed values are sent; the API copies the rest. */
export function correctFormToRequest(
  entry: FinanceEntry,
  values: CorrectFormValues,
): FinanceEntryCorrectRequest {
  const body: FinanceEntryCorrectRequest = { reason: values.reason.trim() };
  if (values.category_uuid !== entry.category.uuid) {
    body.category_uuid = values.category_uuid;
  }
  if (values.cash_account_uuid !== entry.account.uuid) {
    body.cash_account_uuid = values.cash_account_uuid;
  }
  const staff = optionalRef(values.staff_uuid);
  if (staff !== (entry.staff?.uuid ?? null)) body.staff_uuid = staff;
  const amount = normalizeAmount(values.amount);
  if (amount !== entry.amount) body.amount = amount;
  if (values.entry_date !== entry.entry_date) {
    body.entry_date = values.entry_date;
  }
  if (values.description.trim() !== entry.description) {
    body.description = values.description.trim();
  }
  return body;
}

// --------------------------------------------------------- classification

export function createClassifyFormSchema(t: Translate) {
  return z.object({
    category_uuid: z.string().min(1, t("finance.validation.category_required")),
    staff_uuid: z.string(),
    description: descriptionField(t),
  });
}

export type ClassifyFormValues = z.infer<
  ReturnType<typeof createClassifyFormSchema>
>;

export function classifyStaff(value: string) {
  return optionalRef(value);
}

// --------------------------------------------------------------- category

export function createCategoryFormSchema(t: Translate) {
  return z.object({
    kind: z.enum(FINANCE_KINDS as [FinanceKind, ...FinanceKind[]]),
    name: z
      .string()
      .trim()
      .min(1, t("finance.validation.name_required"))
      .max(MAX_CATEGORY_NAME, t("finance.validation.name_max")),
    sort_order: z
      .string()
      .trim()
      .refine(
        (v) => v === "" || /^\d{1,6}$/.test(v),
        t("finance.validation.sort_order"),
      ),
  });
}

export type CategoryFormValues = z.infer<
  ReturnType<typeof createCategoryFormSchema>
>;

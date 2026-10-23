import { z } from "zod";

import {
  CASH_ACCOUNT_KINDS,
  HEAD_OFFICE,
  MAX_DESCRIPTION,
  MAX_NOTE,
  MAX_SORT_ORDER,
} from "@/features/cash/constants";
import { normalizeAmount } from "@/features/cash/lib/format";
import type {
  CashAccount,
  CashAccountCreateRequest,
  CashAccountKind,
  CashClosingCreateRequest,
  CashMovementCreateRequest,
  CashTransferRequest,
} from "@/features/cash/types";

type Translate = (key: string) => string;

const AMOUNT = /^\d+([.,]\d{1,2})?$/;
const SIGNED_AMOUNT = /^-?\d+([.,]\d{1,2})?$/;
const IBAN = /^[A-Z]{2}\d{2}[A-Z0-9]{10,30}$/;

function amountField(t: Translate, positive: boolean) {
  return z
    .string()
    .trim()
    .refine((v) => AMOUNT.test(v), t("cash.validation.amount"))
    .refine(
      (v) => !positive || Number(v.replace(",", ".")) > 0,
      t("cash.validation.amount_positive"),
    );
}

function reasonField(t: Translate) {
  return z
    .string()
    .trim()
    .min(3, t("cash.validation.reason_min"))
    .max(500, t("cash.validation.reason_max"));
}

// ---------------------------------------------------------------- account

export function createAccountFormSchema(t: Translate) {
  return z
    .object({
      branch_uuid: z.string().min(1, t("cash.validation.branch_required")),
      kind: z.enum(CASH_ACCOUNT_KINDS as [string, ...string[]]),
      name: z
        .string()
        .trim()
        .min(1, t("cash.validation.name_required"))
        .max(120, t("cash.validation.name_max")),
      opening_balance: z
        .string()
        .trim()
        .refine((v) => SIGNED_AMOUNT.test(v), t("cash.validation.amount")),
      opening_date: z.string().min(1, t("cash.validation.date_required")),
      bank_name: z.string().trim().max(120, t("cash.validation.name_max")),
      iban: z.string(),
      provider: z.string().trim().max(32, t("cash.validation.provider_max")),
      note: z.string().max(MAX_NOTE),
      sort_order: z
        .string()
        .trim()
        .refine((v) => {
          const n = Number(v);
          return (
            v !== "" && Number.isInteger(n) && n >= 0 && n <= MAX_SORT_ORDER
          );
        }, t("cash.validation.sort_order")),
    })
    .superRefine((values, ctx) => {
      const iban = compactIban(values.iban);
      if (values.kind === "bank" && iban && !IBAN.test(iban)) {
        ctx.addIssue({
          code: "custom",
          path: ["iban"],
          message: t("cash.validation.iban"),
        });
      }
      if (
        values.kind === "cash" &&
        Number(values.opening_balance.replace(",", ".")) < 0
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["opening_balance"],
          message: t("cash.validation.cash_opening_negative"),
        });
      }
    });
}

export type AccountFormValues = z.infer<
  ReturnType<typeof createAccountFormSchema>
>;

function compactIban(value: string) {
  return value.replace(/\s+/g, "").toUpperCase();
}

export function accountFormDefaults(
  account?: CashAccount,
  opts: { branchUuid?: string; today: string } = { today: "" },
): AccountFormValues {
  return {
    branch_uuid: account
      ? (account.branch?.uuid ?? HEAD_OFFICE)
      : (opts.branchUuid ?? ""),
    kind: (account?.kind ?? "cash") as CashAccountKind,
    name: account?.name ?? "",
    opening_balance: account?.opening_balance ?? "0.00",
    opening_date: account?.opening_date ?? opts.today,
    bank_name: account?.bank_name ?? "",
    iban: account?.iban ?? "",
    provider: account?.provider ?? "",
    note: account?.note ?? "",
    sort_order: String(account?.sort_order ?? 0),
  };
}

/**
 * Form → request. Branch and kind are fixed after create and the opening
 * balance / date are locked once the account has movements, so an edit
 * sends only what may still change.
 */
export function accountFormToRequest(
  values: AccountFormValues,
  mode: "create" | "edit",
  locked: boolean,
): CashAccountCreateRequest {
  const kind = values.kind as CashAccountKind;
  const body: CashAccountCreateRequest = {
    kind,
    name: values.name.trim(),
    bank_name: kind === "bank" ? values.bank_name.trim() : "",
    iban: kind === "bank" ? compactIban(values.iban) : "",
    provider: kind === "online" ? values.provider.trim() : "",
    note: values.note,
    sort_order: Number(values.sort_order),
  };
  if (mode === "create" && values.branch_uuid !== HEAD_OFFICE) {
    body.branch_uuid = values.branch_uuid;
  }
  if (!locked) {
    body.opening_balance = normalizeAmount(values.opening_balance);
    body.opening_date = values.opening_date;
  }
  if (mode === "edit") {
    // Kind is immutable; the API refuses a change, so send none.
    const { kind: _kind, ...patch } = body;
    void _kind;
    return patch as CashAccountCreateRequest;
  }
  return body;
}

// ---------------------------------------------------------------- movement

export function createMovementFormSchema(t: Translate) {
  return z.object({
    direction: z.enum(["in", "out"]),
    amount: amountField(t, true),
    movement_date: z.string().min(1, t("cash.validation.date_required")),
    description: z
      .string()
      .trim()
      .min(1, t("cash.validation.description_required"))
      .max(MAX_DESCRIPTION, t("cash.validation.description_max")),
  });
}

export type MovementFormValues = z.infer<
  ReturnType<typeof createMovementFormSchema>
>;

export function movementFormToRequest(
  values: MovementFormValues,
): CashMovementCreateRequest {
  return {
    direction: values.direction,
    amount: normalizeAmount(values.amount),
    movement_date: values.movement_date,
    description: values.description.trim(),
  };
}

// ---------------------------------------------------------------- transfer

export function createTransferFormSchema(t: Translate) {
  return z
    .object({
      from_account_uuid: z
        .string()
        .min(1, t("cash.validation.account_required")),
      to_account_uuid: z.string().min(1, t("cash.validation.account_required")),
      amount: amountField(t, true),
      movement_date: z.string().min(1, t("cash.validation.date_required")),
      description: z
        .string()
        .trim()
        .max(MAX_DESCRIPTION, t("cash.validation.description_max")),
    })
    .refine((v) => v.from_account_uuid !== v.to_account_uuid, {
      path: ["to_account_uuid"],
      message: t("cash.validation.same_account"),
    });
}

export type TransferFormValues = z.infer<
  ReturnType<typeof createTransferFormSchema>
>;

export function transferFormToRequest(
  values: TransferFormValues,
): CashTransferRequest {
  return {
    from_account_uuid: values.from_account_uuid,
    to_account_uuid: values.to_account_uuid,
    amount: normalizeAmount(values.amount),
    movement_date: values.movement_date,
    description: values.description.trim(),
  };
}

// ---------------------------------------------------------------- closing

export function createClosingFormSchema(t: Translate) {
  return z.object({
    business_date: z.string().min(1, t("cash.validation.date_required")),
    counted_amount: amountField(t, false),
    note: z.string().max(MAX_NOTE),
  });
}

export type ClosingFormValues = z.infer<
  ReturnType<typeof createClosingFormSchema>
>;

export function closingFormToRequest(
  values: ClosingFormValues,
): CashClosingCreateRequest {
  return {
    business_date: values.business_date,
    counted_amount: normalizeAmount(values.counted_amount),
    note: values.note,
  };
}

// ---------------------------------------------------------------- reason

export function createReasonFormSchema(t: Translate) {
  return z.object({ reason: reasonField(t) });
}

export type ReasonFormValues = z.infer<
  ReturnType<typeof createReasonFormSchema>
>;

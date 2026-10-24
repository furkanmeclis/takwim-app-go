"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { CASH_POST_KINDS } from "@/features/cash/constants";
import {
  useCashAccountOptions,
  usePostCashLine,
} from "@/features/cash/hooks/use-cash";
import {
  kindLabel,
  movementKindLabel,
  useCashAmount,
} from "@/features/cash/lib/format";
import {
  createPostLineFormSchema,
  postLineFormToRequest,
  type PostLineFormValues,
} from "@/features/cash/schemas/cash-forms";
import type { CashStatementLine } from "@/features/cash/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-post-line-form";

/**
 * Writes a statement line that has no book movement yet: as a movement of
 * the account (income, expense, …) or, for a provider payout, as a transfer
 * with another account. The posting is idempotent per line.
 */
export function PostLineDialog({
  line,
  open,
  onOpenChange,
}: {
  line: CashStatementLine | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  const post = usePostCashLine();
  const options = useCashAccountOptions(open);
  const schema = useMemo(() => createPostLineFormSchema(t), [t]);
  const payout = line?.line_type === "payout";
  const direction = line?.direction ?? "in";

  const accountOptions = useMemo(
    () =>
      (options.data?.items ?? [])
        .filter(
          (a) =>
            a.status === "active" &&
            a.uuid !== line?.account.uuid &&
            (!line || a.currency === line.currency),
        )
        .map((account) => ({
          value: account.uuid,
          label: [
            account.name,
            kindLabel(t, account.kind),
            account.branch_name || t("cash.head_office"),
          ].join(" · "),
        })),
    [line, options.data?.items, t],
  );

  const kindOptions = useMemo(
    () =>
      CASH_POST_KINDS[direction].map((kind) => ({
        value: kind,
        label: movementKindLabel(t, kind),
      })),
    [direction, t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("cash.post_dialog.title")}
      description={
        line
          ? t("cash.post_dialog.description", {
              date: date(line.line_date, undefined, locale),
              amount: money(
                line.direction === "out" ? `-${line.amount}` : line.amount,
                line.currency,
              ),
              description: line.description || line.reference || "—",
            })
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={post.isPending}>
            {t("cash.post_dialog.submit")}
          </Button>
        </>
      }
    >
      {open && line ? (
        <AppForm<PostLineFormValues>
          key={line.uuid}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            mode: payout ? "transfer" : "movement",
            kind: CASH_POST_KINDS[direction][0],
            counter_account_uuid: "",
            description: line.description || line.reference,
            note: "",
          }}
          onSubmit={async (values) => {
            await post.mutateAsync({
              lineUuid: line.uuid,
              body: postLineFormToRequest(values),
            });
            onOpenChange(false);
          }}
        >
          {(form) => {
            const mode = form.watch("mode");
            return (
              <>
                {line.fee && line.fee !== "0.00" ? (
                  <Alert>
                    <AlertDescription>
                      {t("cash.post_dialog.fee_hint", {
                        amount: money(line.fee, line.currency),
                      })}
                    </AlertDescription>
                  </Alert>
                ) : null}
                <AppToggleGroup
                  name="mode"
                  label={t("cash.post_dialog.mode")}
                  description={
                    payout ? t("cash.post_dialog.payout_hint") : undefined
                  }
                  options={[
                    {
                      value: "movement",
                      label: t("cash.post_dialog.mode_movement"),
                    },
                    {
                      value: "transfer",
                      label: t("cash.post_dialog.mode_transfer"),
                    },
                  ]}
                />
                {mode === "transfer" ? (
                  <AppSelect
                    name="counter_account_uuid"
                    label={t(
                      direction === "out"
                        ? "cash.fields.to_account"
                        : "cash.fields.from_account",
                    )}
                    placeholder={t("cash.fields.account_placeholder")}
                    options={accountOptions}
                    description={t("cash.post_dialog.counter_hint")}
                  />
                ) : (
                  <AppSelect
                    name="kind"
                    label={t("cash.post_dialog.kind")}
                    options={kindOptions}
                  />
                )}
                <AppInput
                  name="description"
                  label={t("cash.fields.description")}
                  description={t("cash.post_dialog.description_hint")}
                />
                <AppTextarea name="note" label={t("cash.fields.note")} />
              </>
            );
          }}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

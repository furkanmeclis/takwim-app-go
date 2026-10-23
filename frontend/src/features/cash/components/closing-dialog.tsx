"use client";

import { useEffect, useMemo, useState } from "react";
import { useWatch } from "react-hook-form";

import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppTextarea,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  useCashClosingPreview,
  useCloseCashDay,
} from "@/features/cash/hooks/use-cash";
import {
  amountTone,
  normalizeAmount,
  useBusinessToday,
  useCashAmount,
} from "@/features/cash/lib/format";
import {
  closingFormToRequest,
  createClosingFormSchema,
  type ClosingFormValues,
} from "@/features/cash/schemas/cash-forms";
import type { CashAccount } from "@/features/cash/types";
import { cn } from "@/lib/utils";
import { date, fromMinor, toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-closing-form";

/** Follows the picked day so the preview shows that day's totals. */
function DaySync({ onDay }: { onDay: (day: string) => void }) {
  const value = useWatch({ name: "business_date" }) as string | undefined;
  useEffect(() => {
    if (value) onDay(value);
  }, [onDay, value]);
  return null;
}

function Row({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="flex items-center justify-between gap-4 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className={cn("font-medium tabular-nums", className)}>{value}</span>
    </div>
  );
}

/**
 * Daily closing of a till: the expected amount (opening + in − out of the
 * day) next to the counted cash; the difference is booked and the day is
 * locked.
 */
export function ClosingDialog({
  account,
  open,
  onOpenChange,
}: {
  account: CashAccount | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  const today = useBusinessToday();
  const [day, setDay] = useState(today);
  const close = useCloseCashDay(account?.uuid ?? "");
  const preview = useCashClosingPreview(
    account?.uuid ?? "",
    day,
    open && Boolean(account),
  );
  const schema = useMemo(() => createClosingFormSchema(t), [t]);
  const data = preview.data;
  const currency = account?.currency;

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setDay(today);
        onOpenChange(next);
      }}
      title={t("cash.closing_dialog.title")}
      description={
        account
          ? t("cash.closing_dialog.description", { name: account.name })
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
          <Button
            type="submit"
            form={FORM_ID}
            disabled={close.isPending || !data || data.closed}
          >
            {t("cash.closing_dialog.submit")}
          </Button>
        </>
      }
    >
      {open && account ? (
        <AppForm<ClosingFormValues>
          key={account.uuid}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ business_date: today, counted_amount: "", note: "" }}
          onSubmit={async (values) => {
            await close.mutateAsync(closingFormToRequest(values));
            onOpenChange(false);
          }}
        >
          {(form) => {
            const counted = toMinor(
              normalizeAmount(form.watch("counted_amount") ?? ""),
            );
            const expected = data ? toMinor(data.expected_amount) : null;
            const difference =
              counted != null && expected != null
                ? fromMinor(counted - expected)
                : null;
            return (
              <>
                <DaySync onDay={setDay} />
                <AppDatePicker
                  name="business_date"
                  label={t("cash.fields.business_date")}
                  description={
                    account.last_closed_date
                      ? t("cash.closing_dialog.last_closed", {
                          date: date(
                            account.last_closed_date,
                            undefined,
                            locale,
                          ),
                        })
                      : undefined
                  }
                />
                {preview.isLoading ? (
                  <Loading label={t("common.loading")} />
                ) : null}
                {data?.closed ? (
                  <Alert variant="destructive">
                    <AlertDescription>
                      {t("cash.closing_dialog.already_closed")}
                    </AlertDescription>
                  </Alert>
                ) : null}
                {data ? (
                  <div className="bg-muted/40 space-y-2 rounded-lg border p-4">
                    <Row
                      label={t("cash.closing.opening_amount")}
                      value={money(data.opening_amount, currency)}
                    />
                    <Row
                      label={t("cash.closing.total_in")}
                      value={money(data.total_in, currency)}
                    />
                    <Row
                      label={t("cash.closing.total_out")}
                      value={money(data.total_out, currency)}
                    />
                    <Row
                      label={t("cash.closing.expected_amount")}
                      value={money(data.expected_amount, currency)}
                      className="text-base"
                    />
                    <p className="text-muted-foreground text-xs">
                      {t("cash.closing_dialog.movement_count", {
                        count: data.movement_count,
                      })}
                    </p>
                  </div>
                ) : null}
                <AppInput
                  name="counted_amount"
                  inputMode="decimal"
                  label={t("cash.fields.counted_amount", {
                    currency: currency ?? "",
                  })}
                  description={t("cash.fields.counted_amount_hint")}
                />
                {difference != null ? (
                  <Row
                    label={t("cash.closing.difference")}
                    value={money(difference, currency)}
                    className={amountTone(difference)}
                  />
                ) : null}
                <AppTextarea
                  name="note"
                  label={t("cash.fields.closing_note")}
                  description={t("cash.fields.closing_note_hint")}
                />
              </>
            );
          }}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

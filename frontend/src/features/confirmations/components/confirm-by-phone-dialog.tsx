"use client";

import { Phone } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useConfirmByPhone } from "@/features/confirmations/hooks/use-confirmations";
import { telHref } from "@/features/confirmations/lib/list";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "confirmation-phone-form";
const MAX_NOTE = 500;

/** The booking whose customer confirmed on the phone. */
export type PhoneConfirmTarget = {
  uuid: string;
  number: number;
  version: number;
  customerName: string;
  phone: string | null;
};

/**
 * "Telefonla teyit edildi": the member called the customer, who said they
 * will come. Recorded like the customer's own "Geleceğim" (no more
 * requests, no automatic cancel) with the member and the note in the
 * history. The version read with the row goes along, so an answer that
 * arrived meanwhile is not overwritten.
 */
export function ConfirmByPhoneDialog({
  target,
  onOpenChange,
}: {
  target: PhoneConfirmTarget | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const confirm = useConfirmByPhone();
  const schema = useMemo(
    () =>
      z.object({
        note: z
          .string()
          .max(MAX_NOTE, t("confirmations.phone.note_max", { max: MAX_NOTE })),
      }),
    [t],
  );
  const tel = telHref(target?.phone);

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={onOpenChange}
      title={
        target
          ? t("confirmations.phone.title", { number: target.number })
          : t("confirmations.phone.action")
      }
      description={
        target
          ? t("confirmations.phone.description", {
              customer: target.customerName,
            })
          : undefined
      }
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={confirm.isPending}>
            {t("confirmations.phone.submit")}
          </Button>
        </>
      }
    >
      {target ? (
        <AppForm<{ note: string }>
          key={target.uuid}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ note: "" }}
          onSubmit={async (values) => {
            await confirm.mutateAsync({
              uuid: target.uuid,
              body: {
                note: values.note.trim() || undefined,
                expected_version: target.version,
              },
            });
            onOpenChange(false);
          }}
        >
          {tel ? (
            <Button asChild variant="outline" size="sm" className="w-fit">
              <a href={tel}>
                <Phone className="size-4" />
                {t("confirmations.phone.call", { phone: target.phone ?? "" })}
              </a>
            </Button>
          ) : null}
          <AppTextarea
            name="note"
            label={t("confirmations.phone.note")}
            description={t("confirmations.phone.note_hint")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

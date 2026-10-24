"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { REQUEST_TEXT_CHANNELS } from "@/features/confirmations/constants";
import { useSaveConfirmationTexts } from "@/features/confirmations/hooks/use-confirmations";
import type { ConfirmationTexts } from "@/features/confirmations/types";
import { TemplatesField } from "@/features/customer-notifications/components/notification-event-dialog";
import {
  MAX_BODY,
  MAX_SUBJECT,
} from "@/features/customer-notifications/constants";
import type { NotificationChannelName } from "@/features/customer-notifications/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "confirmation-texts-form";

type TextsFormValues = {
  templates: {
    channel: NotificationChannelName;
    subject: string;
    body: string;
  }[];
};

/**
 * The request's SMS and e-mail texts (TEC-979), with the same editor as
 * the customer notifications: variables, preview and "Varsayılan metin".
 * Every text keeps the visit link ({{link}}); an empty text, or the
 * default one, goes back to the default. The rule (channels, timing) is
 * edited apart.
 */
export function ConfirmationTextsDialog({
  texts,
  open,
  readOnly,
  onClose,
}: {
  texts: ConfirmationTexts;
  open: boolean;
  readOnly: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveConfirmationTexts();

  const schema = useMemo(() => {
    const placeholder = (name: string) =>
      new RegExp(`\\{\\{\\s*${name}\\s*\\}\\}`);
    return z
      .object({
        templates: z.array(
          z.object({
            channel: z.enum(["sms", "whatsapp", "email"]),
            subject: z
              .string()
              .max(
                MAX_SUBJECT,
                t("customer_notifications.validation.subject_too_long"),
              ),
            body: z
              .string()
              .max(
                MAX_BODY,
                t("customer_notifications.validation.body_too_long"),
              ),
          }),
        ),
      })
      .superRefine((values, ctx) => {
        values.templates.forEach((tpl, index) => {
          if (!tpl.body.trim()) return;
          if (tpl.channel === "email" && !tpl.subject.trim()) {
            ctx.addIssue({
              code: "custom",
              path: ["templates", index, "subject"],
              message: t("customer_notifications.validation.subject_required"),
            });
          }
          const missing = texts.required.filter(
            (name) => !placeholder(name).test(tpl.body),
          );
          if (missing.length) {
            ctx.addIssue({
              code: "custom",
              path: ["templates", index, "body"],
              message: t("confirmations.texts.link_required", {
                names: missing.map((name) => `{{${name}}}`).join(", "),
              }),
            });
          }
        });
      });
  }, [t, texts.required]);

  const channels = useMemo(
    () =>
      REQUEST_TEXT_CHANNELS.filter((channel) =>
        texts.templates.some((tpl) => tpl.channel === channel),
      ),
    [texts.templates],
  );

  const defaultValues = useMemo<TextsFormValues>(
    () => ({
      templates: channels.map((channel) => {
        const tpl = texts.templates.find((x) => x.channel === channel);
        return {
          channel,
          subject: tpl?.subject ?? "",
          body: tpl?.body ?? "",
        };
      }),
    }),
    [channels, texts.templates],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("confirmations.texts.dialog_title")}
      description={t("confirmations.texts.dialog_description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {readOnly ? t("common.close") : t("form.cancel")}
          </Button>
          {readOnly ? null : (
            <Button type="submit" form={FORM_ID} disabled={save.isPending}>
              {t("customer_notifications.editor.save")}
            </Button>
          )}
        </>
      }
    >
      {open ? (
        <AppForm<TextsFormValues>
          key={texts.updated_at ?? "default"}
          id={FORM_ID}
          schema={schema}
          defaultValues={defaultValues}
          onSubmit={async (values) => {
            await save.mutateAsync({
              templates: values.templates.map((tpl) => ({
                channel: tpl.channel as "sms" | "email",
                body: tpl.body.trim(),
                ...(tpl.channel === "email"
                  ? { subject: tpl.subject.trim() }
                  : {}),
              })),
            });
            onClose();
          }}
        >
          <TemplatesField
            event={{
              code: texts.event,
              variables: texts.variables,
              templates: texts.templates,
            }}
            channels={channels}
            hint={t("confirmations.texts.editor_hint")}
            disabled={readOnly}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

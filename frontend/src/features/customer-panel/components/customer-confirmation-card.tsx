"use client";

import { CalendarCheck, CheckCircle2, MessageSquare } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntitySectionCard } from "@/components/entity";
import { AppForm, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import type { CustomerReservation } from "@/features/customer-panel/types";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-change-request-form";
const MAX_NOTE = 500;

type ChangeValues = { note: string };

/**
 * The business's confirmation request (TEC-921): "Geleceğim" and "Değişiklik
 * iste" while the API offers them, with the deadline; afterwards what the
 * person answered. Cancelling stays the ordinary cancel action. Opening the
 * page answers nothing; only these buttons do.
 */
export function CustomerConfirmationCard({
  reservation,
  confirming,
  requesting,
  onConfirm,
  onRequestChange,
}: {
  reservation: CustomerReservation;
  confirming: boolean;
  requesting: boolean;
  onConfirm: () => void;
  /** Resolves true once the request is sent, so the dialog can close. */
  onRequestChange: (note: string) => Promise<boolean>;
}) {
  const { t, locale } = useLocale();
  const [changing, setChanging] = useState(false);
  const schema = useMemo(
    () =>
      z.object({
        note: z
          .string()
          .max(MAX_NOTE, t("customer_panel.confirmation.note_too_long")),
      }),
    [t],
  );
  const c = reservation.confirmation;
  const { confirm, request_change: change } = reservation.actions;
  if (!c) return null;

  const when = (iso: string) =>
    datetimeInZone(iso, reservation.timezone, locale, {
      dateStyle: "medium",
      timeStyle: "short",
    });

  if (!confirm.allowed && !change.allowed) {
    if (c.status === "confirmed") {
      return (
        <Alert>
          <CheckCircle2 className="size-4" />
          <AlertDescription>
            {t("customer_panel.confirmation.confirmed")}
          </AlertDescription>
        </Alert>
      );
    }
    if (c.change_requested_at && reservation.status === "active") {
      return (
        <Alert>
          <MessageSquare className="size-4" />
          <AlertDescription>
            {t("customer_panel.confirmation.change_sent")}
          </AlertDescription>
        </Alert>
      );
    }
    return null;
  }

  const until = confirm.until ?? change.until;
  return (
    <EntitySectionCard title={t("customer_panel.confirmation.title")}>
      <div className="space-y-4">
        <p className="text-sm">
          {until
            ? t("customer_panel.confirmation.ask_until", { at: when(until) })
            : t("customer_panel.confirmation.ask")}
        </p>
        {c.change_requested_at ? (
          <Alert>
            <MessageSquare className="size-4" />
            <AlertTitle>
              {t("customer_panel.confirmation.change_sent")}
            </AlertTitle>
            <AlertDescription>
              {t("customer_panel.confirmation.change_sent_hint")}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-col gap-2 sm:flex-row">
          {confirm.allowed ? (
            <Button type="button" disabled={confirming} onClick={onConfirm}>
              <CalendarCheck className="size-4" />
              {t("customer_panel.actions.confirm")}
            </Button>
          ) : null}
          {change.allowed ? (
            <Button
              type="button"
              variant="outline"
              disabled={requesting}
              onClick={() => setChanging(true)}
            >
              <MessageSquare className="size-4" />
              {t("customer_panel.actions.request_change")}
            </Button>
          ) : null}
        </div>
        {reservation.actions.cancel.allowed ? (
          <p className="text-muted-foreground text-xs">
            {t("customer_panel.confirmation.cancel_hint")}
          </p>
        ) : null}
      </div>

      <AppDialog
        open={changing}
        onOpenChange={setChanging}
        title={t("customer_panel.confirmation.change_title")}
        description={t("customer_panel.confirmation.change_description")}
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setChanging(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={requesting}>
              {t("customer_panel.confirmation.change_send")}
            </Button>
          </>
        }
      >
        {changing ? (
          <AppForm<ChangeValues>
            id={FORM_ID}
            schema={schema}
            defaultValues={{ note: "" }}
            onSubmit={async (values) => {
              if (await onRequestChange(values.note.trim())) {
                setChanging(false);
              }
            }}
          >
            <AppTextarea
              name="note"
              label={t("customer_panel.confirmation.note")}
              description={t("customer_panel.confirmation.note_hint")}
              rows={4}
              maxLength={MAX_NOTE}
            />
          </AppForm>
        ) : null}
      </AppDialog>
    </EntitySectionCard>
  );
}

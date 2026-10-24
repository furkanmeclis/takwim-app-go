"use client";

import { Phone, TriangleAlert } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppTextarea,
  FormActions,
} from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useReassignCustomerPhone } from "@/features/customer-phones/hooks/use-customer-phones";
import { emitApiError, isApiError } from "@/lib/api";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (key: string) => string;

function reassignmentSchema(t: Translate) {
  return z.object({
    phone: z
      .string()
      .trim()
      .min(7, t("customer_phones.form.invalid_phone"))
      .max(32, t("customer_phones.form.invalid_phone")),
    note: z.string().trim().max(500, t("customer_phones.form.note_too_long")),
  });
}

type ReassignmentValues = z.infer<ReturnType<typeof reassignmentSchema>>;

type ReassignmentDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/**
 * A reassignment notice of a number (operator notice, support request),
 * processed at once after a confirmation: the earlier history of the
 * number's profile is hidden at every business and its sessions end.
 */
export function ReassignmentDrawer({
  open,
  onOpenChange,
}: ReassignmentDrawerProps) {
  const { t } = useLocale();
  const { confirm } = useDialogs();
  const reassign = useReassignCustomerPhone();
  const schema = useMemo(() => reassignmentSchema(t), [t]);

  const submit = async (values: ReassignmentValues) => {
    const confirmed = await confirm({
      title: t("customer_phones.confirm.title"),
      description: t("customer_phones.confirm.description"),
      confirmLabel: t("customer_phones.confirm.action"),
      variant: "destructive",
    });
    if (!confirmed) return;
    try {
      const release = await reassign.mutateAsync({
        phone: values.phone,
        note: values.note,
      });
      appToast.success(
        release.has_profile
          ? t("customer_phones.toast.processed", {
              businesses: release.businesses_hidden,
              sessions: release.sessions_revoked,
            })
          : t("customer_phones.toast.processed_no_profile"),
      );
      onOpenChange(false);
    } catch (error) {
      if (!isApiError(error)) return;
      if (error.code === "PHONE_ALREADY_RELEASED") {
        appToast.error(t("customer_phones.errors.already_released"));
        return;
      }
      if (error.details.some((d) => d.field === "phone")) {
        appToast.error(t("customer_phones.form.invalid_phone"));
        return;
      }
      emitApiError(error);
    }
  };

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={t("customer_phones.form.title")}
      description={t("customer_phones.form.description")}
      size="md"
    >
      <AppForm<ReassignmentValues>
        key={open ? "open" : "closed"}
        schema={schema}
        defaultValues={{ phone: "", note: "" }}
        onSubmit={submit}
      >
        {() => (
          <div className="space-y-4">
            <Alert>
              <TriangleAlert className="size-4" />
              <AlertTitle>{t("customer_phones.form.effects_title")}</AlertTitle>
              <AlertDescription>
                <ul className="list-disc space-y-1 ps-4">
                  <li>{t("customer_phones.form.effect_history")}</li>
                  <li>{t("customer_phones.form.effect_sessions")}</li>
                  <li>{t("customer_phones.form.effect_records")}</li>
                </ul>
              </AlertDescription>
            </Alert>
            <AppInput
              name="phone"
              type="tel"
              inputMode="tel"
              autoComplete="off"
              startIcon={Phone}
              label={t("customer_phones.form.phone")}
              placeholder={t("customer_phones.form.phone_placeholder")}
            />
            <AppTextarea
              name="note"
              rows={3}
              label={t("customer_phones.form.note")}
              description={t("customer_phones.form.note_hint")}
            />
            <FormActions>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                variant="destructive"
                disabled={reassign.isPending}
              >
                {t("customer_phones.form.submit")}
              </Button>
            </FormActions>
          </div>
        )}
      </AppForm>
    </EntityDrawer>
  );
}

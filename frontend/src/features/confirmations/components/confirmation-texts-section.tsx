"use client";

import { Pencil, RotateCcw } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmationTextsDialog } from "@/features/confirmations/components/confirmation-texts-dialog";
import {
  useConfirmationTexts,
  useResetConfirmationTexts,
} from "@/features/confirmations/hooks/use-confirmations";
import type { ConfirmationTexts } from "@/features/confirmations/types";
import { channelLabel } from "@/features/customer-notifications/constants";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Teyit mesajı" (TEC-979): what the request says on each channel. SMS
 * and e-mail are the business's texts (or the defaults); WhatsApp sends
 * the template approved in the business's Meta account and the automatic
 * call reads the standard text, so both are shown read-only.
 */
export function ConfirmationTextsSection({ canWrite }: { canWrite: boolean }) {
  const { t } = useLocale();
  const texts = useConfirmationTexts();
  const reset = useResetConfirmationTexts();
  const [editing, setEditing] = useState(false);
  const [resetting, setResetting] = useState(false);
  const data = texts.data;

  return (
    <EntitySectionCard
      title={t("confirmations.texts.title")}
      actions={
        data ? (
          <div className="flex flex-wrap gap-2">
            {canWrite && data.custom ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setResetting(true)}
              >
                <RotateCcw className="size-4" />
                {t("confirmations.texts.reset")}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant={canWrite ? "default" : "outline"}
              onClick={() => setEditing(true)}
            >
              <Pencil className="size-4" />
              {t(
                canWrite
                  ? "confirmations.texts.edit"
                  : "confirmations.actions.view",
              )}
            </Button>
          </div>
        ) : null
      }
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("confirmations.texts.hint")}
      </p>
      {texts.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : texts.isError || !data ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("confirmations.texts.load_failed")}
          onRetry={() => void texts.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <>
          <TextsSummary texts={data} />
          <ConfirmationTextsDialog
            texts={data}
            open={editing}
            readOnly={!canWrite}
            onClose={() => setEditing(false)}
          />
        </>
      )}
      <ConfirmDialog
        open={resetting}
        title={t("confirmations.texts.reset_title")}
        description={t("confirmations.texts.reset_description")}
        confirmLabel={t("confirmations.texts.reset")}
        isPending={reset.isPending}
        onConfirm={() =>
          reset.mutate(undefined, { onSuccess: () => setResetting(false) })
        }
        onCancel={() => setResetting(false)}
      />
    </EntitySectionCard>
  );
}

function TextsSummary({ texts }: { texts: ConfirmationTexts }) {
  const { t } = useLocale();
  return (
    <EntityDetail
      sections={[
        {
          id: "texts",
          fields: [
            ...texts.templates.map((tpl) => ({
              key: tpl.channel,
              label: channelLabel(t, tpl.channel),
              value: (
                <div className="min-w-0 space-y-1">
                  <Badge variant={tpl.custom ? "outline" : "secondary"}>
                    {t(
                      tpl.custom
                        ? "customer_notifications.editor.custom_text"
                        : "customer_notifications.editor.default_text",
                    )}
                  </Badge>
                  {tpl.channel === "email" && tpl.subject ? (
                    <p className="font-medium break-words">{tpl.subject}</p>
                  ) : null}
                  <p className="text-muted-foreground line-clamp-3 text-sm break-words whitespace-pre-wrap">
                    {tpl.body}
                  </p>
                </div>
              ),
            })),
            {
              key: "whatsapp",
              label: channelLabel(t, "whatsapp"),
              value: (
                <p className="text-muted-foreground text-sm">
                  {t("confirmations.texts.whatsapp_note")}
                </p>
              ),
            },
            {
              key: "voice_call",
              label: channelLabel(t, "voice_call"),
              value: (
                <div className="min-w-0 space-y-1">
                  <p className="text-muted-foreground text-sm">
                    {t("confirmations.texts.voice_note")}
                  </p>
                  {texts.voice ? (
                    <p className="text-sm break-words whitespace-pre-wrap">
                      {texts.voice}
                    </p>
                  ) : null}
                </div>
              ),
            },
          ],
        },
      ]}
    />
  );
}

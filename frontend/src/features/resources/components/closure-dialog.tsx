"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppDatePicker, AppForm, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useSaveClosure } from "@/features/resources/hooks/use-resources";
import {
  closureFormDefaults,
  closureFormToRequest,
  createClosureSchema,
  type ClosureFormValues,
} from "@/features/resources/schemas/resource-form";
import type { Closure } from "@/features/resources/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "resource-closure-form";

/** Create / edit a closed period (maintenance, private event …). */
export function ClosureDialog({
  resourceUuid,
  open,
  closure,
  onOpenChange,
}: {
  resourceUuid: string;
  open: boolean;
  /** Period to edit; `null` creates a new one. */
  closure: Closure | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveClosure(resourceUuid);
  const schema = useMemo(() => createClosureSchema(t), [t]);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        closure
          ? t("resources.closures.edit_title")
          : t("resources.closures.create_title")
      }
      description={t("resources.closures.dialog_description")}
      className="sm:max-w-xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {closure
              ? t("resources.actions.save")
              : t("resources.closures.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ClosureFormValues>
          key={closure?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={closureFormDefaults(closure ?? undefined)}
          onSubmit={async (values) => {
            await save.mutateAsync({
              closureUuid: closure?.uuid,
              body: closureFormToRequest(values),
            });
            onOpenChange(false);
          }}
        >
          <AppDatePicker
            name="starts_at"
            withTime
            label={t("resources.closures.fields.starts_at")}
          />
          <AppDatePicker
            name="ends_at"
            withTime
            label={t("resources.closures.fields.ends_at")}
          />
          <AppTextarea
            name="reason"
            label={t("resources.closures.fields.reason")}
            description={t("resources.closures.fields.reason_hint")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

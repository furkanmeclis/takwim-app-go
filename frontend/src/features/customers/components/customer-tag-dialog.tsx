"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  createTagSchema,
  type TagFormValues,
} from "@/features/customers/schemas/customer-form";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-bulk-tag";

/**
 * Asks the tag of the bulk `add_tag` / `remove_tag` actions. Tags already
 * in use are offered as shortcuts.
 */
export function CustomerTagDialog({
  open,
  mode,
  suggestions,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  mode: "add" | "remove";
  suggestions: string[];
  onSubmit: (tag: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createTagSchema(t), [t]);

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
      title={
        mode === "add"
          ? t("customers.bulk_tag.add_title")
          : t("customers.bulk_tag.remove_title")
      }
      description={t("customers.bulk_tag.description")}
      footer={
        <>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID}>
            {t("customers.bulk_tag.continue")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<TagFormValues>
          id={FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ tag: "" }}
          onSubmit={(values) => onSubmit(values.tag.trim())}
        >
          {(form) => (
            <>
              <AppInput name="tag" label={t("customers.fields.tag")} />
              {suggestions.length > 0 ? (
                <div className="flex flex-wrap gap-1.5">
                  {suggestions.map((tag) => (
                    <Button
                      key={tag}
                      type="button"
                      size="sm"
                      variant="ghost"
                      className="h-auto p-0"
                      onClick={() =>
                        form.setValue("tag", tag, { shouldValidate: true })
                      }
                    >
                      <Badge variant="secondary">{tag}</Badge>
                    </Button>
                  ))}
                </div>
              ) : null}
            </>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

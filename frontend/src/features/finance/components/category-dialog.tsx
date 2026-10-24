"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { FINANCE_KINDS } from "@/features/finance/constants";
import { useSaveFinanceCategory } from "@/features/finance/hooks/use-finance";
import { kindLabel } from "@/features/finance/lib/format";
import {
  createCategoryFormSchema,
  type CategoryFormValues,
} from "@/features/finance/schemas/finance-forms";
import type { FinanceCategory, FinanceKind } from "@/features/finance/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "finance-category-form";

/** Creates a category or renames / reorders one; the kind is fixed after create. */
export function CategoryDialog({
  open,
  category,
  defaultKind,
  onOpenChange,
}: {
  open: boolean;
  category: FinanceCategory | null;
  defaultKind: FinanceKind;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveFinanceCategory();
  const schema = useMemo(() => createCategoryFormSchema(t), [t]);
  const kindOptions = FINANCE_KINDS.map((value) => ({
    value,
    label: kindLabel(t, value),
  }));

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t(
        category
          ? "finance.category_dialog.title_edit"
          : "finance.category_dialog.title_create",
      )}
      description={t("finance.category_dialog.description")}
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
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {t("form.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<CategoryFormValues>
          key={category?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            kind: category?.kind ?? defaultKind,
            name: category?.name ?? "",
            sort_order: category ? String(category.sort_order) : "",
          }}
          onSubmit={async (values) => {
            const sortOrder =
              values.sort_order === "" ? undefined : Number(values.sort_order);
            if (category) {
              await save.mutateAsync({
                uuid: category.uuid,
                patch: { name: values.name.trim(), sort_order: sortOrder },
              });
            } else {
              await save.mutateAsync({
                create: {
                  kind: values.kind,
                  name: values.name.trim(),
                  sort_order: sortOrder,
                },
              });
            }
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="kind"
            label={t("finance.fields.kind")}
            options={kindOptions}
            disabled={Boolean(category)}
          />
          <AppInput
            name="sort_order"
            inputMode="numeric"
            label={t("finance.fields.sort_order")}
            description={t("finance.fields.sort_order_hint")}
          />
          <AppInput
            name="name"
            label={t("finance.fields.name")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

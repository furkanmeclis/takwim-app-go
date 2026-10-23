"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  useCreateServiceCategory,
  useUpdateServiceCategory,
} from "@/features/services/hooks/use-services";
import {
  createCategorySchema,
  type CategoryFormValues,
} from "@/features/services/schemas/service-form";
import type { ServiceCategory } from "@/features/services/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "service-category-form";

/** Create / rename a service category. */
export function ServiceCategoryDialog({
  open,
  category,
  onOpenChange,
}: {
  open: boolean;
  /** Category to edit; `null` creates one. */
  category: ServiceCategory | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createCategorySchema(t), [t]);
  const create = useCreateServiceCategory();
  const update = useUpdateServiceCategory();
  const pending = create.isPending || update.isPending;

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        category
          ? t("services.categories.edit_title")
          : t("services.categories.create_title")
      }
      description={t("services.categories.dialog_description")}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={pending}>
            {category
              ? t("services.actions.save")
              : t("services.categories.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<CategoryFormValues>
          key={category?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            name: category?.name ?? "",
            description: category?.description ?? "",
            sort_order: String(category?.sort_order ?? 0),
          }}
          onSubmit={async (values) => {
            const body = {
              name: values.name.trim(),
              description: values.description.trim(),
              sort_order: Number(values.sort_order),
            };
            if (category) {
              await update.mutateAsync({ uuid: category.uuid, body });
            } else {
              await create.mutateAsync(body);
            }
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("services.categories.fields.name")} />
          <AppInput
            name="sort_order"
            type="number"
            min={0}
            label={t("services.fields.sort_order")}
            description={t("services.categories.fields.sort_order_hint")}
          />
          <AppTextarea
            name="description"
            label={t("services.fields.description")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

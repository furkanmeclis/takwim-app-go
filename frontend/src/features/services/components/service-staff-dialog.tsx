"use client";

import { useCallback, useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppCombobox, AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { MAX_DURATION, MIN_DURATION } from "@/features/services/constants";
import {
  createStaffLinkSchema,
  type StaffLinkFormValues,
} from "@/features/services/schemas/service-form";
import type { ServiceStaff } from "@/features/services/types";
import { staffService } from "@/features/staff/services/staff.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "service-staff-form";

/**
 * Adds a staff member who may perform the service, or edits their personal
 * price / duration (empty = the service's).
 */
export function ServiceStaffDialog({
  open,
  link,
  linkedUuids,
  catalogPrice,
  catalogDuration,
  isPending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Link to edit; `null` adds a staff member. */
  link: ServiceStaff | null;
  /** Staff already linked (hidden from the picker). */
  linkedUuids: string[];
  catalogPrice: string;
  catalogDuration: number;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: StaffLinkFormValues) => Promise<void>;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const schema = useMemo(() => createStaffLinkSchema(t), [t]);

  const loadStaff = useCallback(
    async (q: string) => {
      const page = await staffService.list(slug, {
        limit: 20,
        offset: 0,
        q: q || undefined,
        is_active: "true",
      });
      return page.items
        .filter((member) => !linkedUuids.includes(member.uuid))
        .map((member) => ({
          value: member.uuid,
          label: member.name,
          description: member.title || undefined,
        }));
    },
    [linkedUuids, slug],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        link
          ? t("services.staff.edit_title", { name: link.name })
          : t("services.staff.add_title")
      }
      description={t("services.staff.dialog_description")}
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
          <Button type="submit" form={FORM_ID} disabled={isPending}>
            {link ? t("services.actions.save") : t("services.staff.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<StaffLinkFormValues>
          key={link?.staff_uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            staff_uuid: link?.staff_uuid ?? "",
            price: link?.price ?? "",
            duration_minutes:
              link?.duration_minutes != null
                ? String(link.duration_minutes)
                : "",
          }}
          onSubmit={async (values) => {
            await onSubmit(values);
            onOpenChange(false);
          }}
        >
          <AppCombobox
            name="staff_uuid"
            label={t("services.staff.fields.staff")}
            placeholder={t("services.staff.fields.staff_placeholder")}
            searchPlaceholder={t("services.staff.fields.staff_search")}
            emptyText={t("services.staff.fields.staff_empty")}
            disabled={Boolean(link)}
            loadOptions={loadStaff}
            initialOptions={
              link ? [{ value: link.staff_uuid, label: link.name }] : undefined
            }
            className="sm:col-span-2"
          />
          <AppInput
            name="price"
            inputMode="decimal"
            label={t("services.staff.fields.price")}
            placeholder={catalogPrice}
            description={t("services.staff.fields.price_hint")}
          />
          <AppInput
            name="duration_minutes"
            type="number"
            min={MIN_DURATION}
            max={MAX_DURATION}
            label={t("services.staff.fields.duration")}
            placeholder={String(catalogDuration)}
            description={t("services.staff.fields.duration_hint")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

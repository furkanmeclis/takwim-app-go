"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSelect, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { MAX_DURATION, MIN_DURATION } from "@/features/services/constants";
import {
  createOfferSchema,
  type OfferFormValues,
} from "@/features/services/schemas/service-form";
import type { ServiceOffer } from "@/features/services/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "service-offer-form";

/**
 * Adds a branch to a service or edits its branch price / duration. Empty
 * price or duration means the catalog value.
 */
export function ServiceOfferDialog({
  open,
  offer,
  branchOptions,
  catalogPrice,
  catalogDuration,
  isPending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Offer to edit; `null` adds a branch. */
  offer: ServiceOffer | null;
  /** Branches the member may add (in scope, not offering it yet). */
  branchOptions: { value: string; label: string }[];
  catalogPrice: string;
  catalogDuration: number;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: OfferFormValues) => Promise<void>;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createOfferSchema(t), [t]);
  const options = offer
    ? [
        {
          value: offer.branch_uuid,
          label: `${offer.branch_name} (${offer.branch_code})`,
        },
      ]
    : branchOptions;

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        offer
          ? t("services.offers.edit_title", { branch: offer.branch_name })
          : t("services.offers.add_title")
      }
      description={t("services.offers.dialog_description")}
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
            {offer ? t("services.actions.save") : t("services.offers.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<OfferFormValues>
          key={offer?.branch_uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            branch_uuid: offer?.branch_uuid ?? options[0]?.value ?? "",
            price: offer?.price ?? "",
            duration_minutes:
              offer?.duration_minutes != null
                ? String(offer.duration_minutes)
                : "",
            is_active: offer?.is_active ?? true,
          }}
          onSubmit={async (values) => {
            await onSubmit(values);
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="branch_uuid"
            label={t("services.offers.fields.branch")}
            options={options}
            disabled={Boolean(offer)}
            className="sm:col-span-2"
          />
          <AppInput
            name="price"
            inputMode="decimal"
            label={t("services.offers.fields.price")}
            placeholder={catalogPrice}
            description={t("services.offers.fields.price_hint")}
          />
          <AppInput
            name="duration_minutes"
            type="number"
            min={MIN_DURATION}
            max={MAX_DURATION}
            label={t("services.offers.fields.duration")}
            placeholder={String(catalogDuration)}
            description={t("services.offers.fields.duration_hint")}
          />
          <AppSwitch
            name="is_active"
            label={t("services.offers.fields.is_active")}
            description={t("services.offers.fields.is_active_hint")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

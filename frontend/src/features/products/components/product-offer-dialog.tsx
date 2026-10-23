"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSelect, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  createOfferSchema,
  type OfferFormValues,
} from "@/features/products/schemas/product-form";
import type { ProductOffer } from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "product-offer-form";

/**
 * Adds a branch selling the product or edits its branch price. Empty price
 * means the catalog price.
 */
export function ProductOfferDialog({
  open,
  offer,
  branchOptions,
  catalogPrice,
  isPending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Offer to edit; `null` adds a branch. */
  offer: ProductOffer | null;
  /** Branches the member may add (in scope, not selling it yet). */
  branchOptions: { value: string; label: string }[];
  catalogPrice: string;
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
          ? t("products.offers.edit_title", { branch: offer.branch_name })
          : t("products.offers.add_title")
      }
      description={t("products.offers.dialog_description")}
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
            {offer ? t("products.actions.save") : t("products.offers.add")}
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
            is_active: offer?.is_active ?? true,
          }}
          onSubmit={async (values) => {
            await onSubmit(values);
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="branch_uuid"
            label={t("products.offers.fields.branch")}
            options={options}
            disabled={Boolean(offer)}
          />
          <AppInput
            name="price"
            inputMode="decimal"
            label={t("products.offers.fields.price")}
            placeholder={catalogPrice}
            description={t("products.offers.fields.price_hint")}
          />
          <AppSwitch
            name="is_active"
            label={t("products.offers.fields.is_active")}
            description={t("products.offers.fields.is_active_hint")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

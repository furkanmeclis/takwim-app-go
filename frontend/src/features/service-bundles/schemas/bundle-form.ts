import { z } from "zod";

import {
  MAX_BUNDLE_DESCRIPTION,
  MAX_BUNDLE_ITEMS,
  MAX_BUNDLE_NAME,
  MAX_BUNDLE_SORT_ORDER,
  MIN_BUNDLE_ITEMS,
} from "@/features/service-bundles/constants";
import type {
  ServiceBundle,
  ServiceBundleCreateRequest,
} from "@/features/service-bundles/types";
import {
  normalizePrice,
  priceField,
} from "@/features/services/schemas/service-form";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export function createBundleFormSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("service_bundles.validation.name_required"))
      .max(MAX_BUNDLE_NAME, t("service_bundles.validation.name_max")),
    description: z.string().max(MAX_BUNDLE_DESCRIPTION),
    price: priceField(t),
    // The contents in the order the customer is served; a service may repeat.
    items: z
      .array(
        z.object({
          service_uuid: z
            .string()
            .min(1, t("service_bundles.validation.service_required")),
        }),
      )
      .min(
        MIN_BUNDLE_ITEMS,
        t("service_bundles.validation.items_min", { min: MIN_BUNDLE_ITEMS }),
      )
      .max(
        MAX_BUNDLE_ITEMS,
        t("service_bundles.validation.items_max", { max: MAX_BUNDLE_ITEMS }),
      ),
    branch_uuids: z.array(z.string()),
    sort_order: z
      .string()
      .trim()
      .refine((v) => {
        const n = Number(v);
        return (
          v !== "" &&
          Number.isInteger(n) &&
          n >= 0 &&
          n <= MAX_BUNDLE_SORT_ORDER
        );
      }, t("service_bundles.validation.sort_order")),
    show_online: z.boolean(),
    is_active: z.boolean(),
  });
}

export type BundleFormValues = z.infer<
  ReturnType<typeof createBundleFormSchema>
>;

export function bundleFormDefaults(
  bundle?: ServiceBundle,
  defaults?: { branchUuids?: string[] },
): BundleFormValues {
  return {
    name: bundle?.name ?? "",
    description: bundle?.description ?? "",
    price: bundle?.price ?? "",
    items: bundle
      ? bundle.items.map((item) => ({ service_uuid: item.service_uuid }))
      : [{ service_uuid: "" }, { service_uuid: "" }],
    // Only the branches the member manages are sent; others stay.
    branch_uuids: bundle
      ? bundle.branches
          .filter((branch) => branch.editable)
          .map((branch) => branch.uuid)
      : (defaults?.branchUuids ?? []),
    sort_order: String(bundle?.sort_order ?? 0),
    show_online: bundle?.show_online ?? true,
    is_active: bundle?.is_active ?? true,
  };
}

export function bundleFormToRequest(
  values: BundleFormValues,
): ServiceBundleCreateRequest {
  return {
    name: values.name.trim(),
    description: values.description.trim(),
    price: normalizePrice(values.price) ?? "0.00",
    service_uuids: values.items.map((item) => item.service_uuid),
    branch_uuids: values.branch_uuids,
    sort_order: Number(values.sort_order),
    show_online: values.show_online,
    is_active: values.is_active,
  };
}

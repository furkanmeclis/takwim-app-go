"use client";

import { Minus, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ButtonGroup, ButtonGroupText } from "@/components/ui/button-group";
import { Checkbox } from "@/components/ui/checkbox";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { productKey } from "@/features/business-page/lib/wizard";
import type { PublicProduct } from "@/features/business-page/types";
import { priceLabel } from "@/features/services/lib/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Optional product step (TEC-919): products the business sells online in
 * the branch, each with a quantity up to `max_quantity` (the API caps it
 * and never shows the exact stock). Picking nothing is fine; the units are
 * reserved only when the booking is made.
 */
export function ProductStep({
  products,
  value,
  onChange,
  currency,
}: {
  products: PublicProduct[];
  /** Quantity by `productKey`; a missing key = not picked. */
  value: Record<string, number>;
  onChange: (value: Record<string, number>) => void;
  currency: string;
}) {
  const { t, locale } = useLocale();

  const setQuantity = (key: string, quantity: number) => {
    const next = { ...value };
    if (quantity > 0) next[key] = quantity;
    else delete next[key];
    onChange(next);
  };

  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-sm">
        {t("business_page.products.hint")}
      </p>
      <div className="grid gap-2">
        {products.map((product) => {
          const key = productKey(product);
          const id = `product-${key}`;
          const quantity = value[key] ?? 0;
          const name = product.variant_name
            ? `${product.name} / ${product.variant_name}`
            : product.name;
          return (
            <div key={key} className="space-y-2">
              <ChoiceCard
                id={id}
                title={name}
                aside={priceLabel(product.price, currency, locale)}
                control={
                  <Checkbox
                    id={id}
                    checked={quantity > 0}
                    onCheckedChange={(next) =>
                      setQuantity(key, next === true ? 1 : 0)
                    }
                  />
                }
              />
              {quantity > 0 ? (
                <ButtonGroup
                  className="ms-auto"
                  aria-label={t("business_page.products.quantity", { name })}
                >
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label={t("business_page.products.less")}
                    onClick={() => setQuantity(key, quantity - 1)}
                  >
                    <Minus className="size-4" />
                  </Button>
                  <ButtonGroupText className="min-w-12 justify-center tabular-nums">
                    {quantity}
                  </ButtonGroupText>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label={t("business_page.products.more")}
                    disabled={quantity >= product.max_quantity}
                    onClick={() => setQuantity(key, quantity + 1)}
                  >
                    <Plus className="size-4" />
                  </Button>
                </ButtonGroup>
              ) : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}

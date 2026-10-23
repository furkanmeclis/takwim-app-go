"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  STOCK_STATUS_LABEL_KEYS,
  STOCK_STATUS_TONES,
} from "@/features/products/constants";
import type { ProductStock } from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";

/** "12 · In stock" with the tone of the status (available units). */
export function StockStatusChip({
  stock,
  showCount = true,
}: {
  stock: Pick<ProductStock, "available" | "status">;
  showCount?: boolean;
}) {
  const { t, locale } = useLocale();
  const label = t(STOCK_STATUS_LABEL_KEYS[stock.status]);
  return (
    <StatusChip
      label={
        showCount
          ? `${stock.available.toLocaleString(locale)} · ${label}`
          : label
      }
      tone={STOCK_STATUS_TONES[stock.status]}
    />
  );
}

"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useCustomerProfile } from "@/features/customer-panel/hooks/use-customer-panel";
import { useLocale } from "@/providers/locale-provider";

const ALL = "all";

/** Narrows a list to one business; hidden when the person has only one. */
export function BusinessSelect({
  value,
  onChange,
}: {
  value: string | undefined;
  onChange: (business: string | undefined) => void;
}) {
  const { t } = useLocale();
  const businesses = useCustomerProfile().data?.businesses ?? [];
  if (businesses.length < 2) return null;

  return (
    <Select
      value={value ?? ALL}
      onValueChange={(next) => onChange(next === ALL ? undefined : next)}
    >
      <SelectTrigger
        size="sm"
        className="w-44"
        aria-label={t("customer_panel.filters.business")}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>
          {t("customer_panel.filters.all_businesses")}
        </SelectItem>
        {businesses.map((business) => (
          <SelectItem key={business.uuid} value={business.uuid}>
            {business.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

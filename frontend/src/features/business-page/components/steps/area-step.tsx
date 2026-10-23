"use client";

import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { useLocale } from "@/providers/locale-provider";

/** Radio value of "no preference" (any area; the API's empty zone). */
const ANY = "__any__";

/**
 * Area step (restaurant): the branch's areas (zones of its online tables)
 * or no preference. The table itself is assigned by the business / system.
 */
export function AreaStep({
  zones,
  value,
  onChange,
}: {
  zones: string[];
  /** Picked area; empty = no preference. */
  value: string;
  onChange: (zone: string) => void;
}) {
  const { t } = useLocale();
  return (
    <RadioGroup
      value={value || ANY}
      onValueChange={(next) => onChange(next === ANY ? "" : next)}
      aria-label={t("business_page.steps.area")}
    >
      <ChoiceCard
        id="area-any"
        title={t("business_page.area.any")}
        description={t("business_page.area.any_hint")}
        control={<RadioGroupItem value={ANY} id="area-any" />}
      />
      {zones.map((zone, index) => (
        <ChoiceCard
          key={zone}
          id={`area-${index}`}
          title={zone}
          control={<RadioGroupItem value={zone} id={`area-${index}`} />}
        />
      ))}
    </RadioGroup>
  );
}

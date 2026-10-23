"use client";

import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { WEEKDAYS } from "@/features/business-page/constants";
import type { PublicBranch } from "@/features/business-page/types";
import { dateKeyInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Today's hours of a branch in its own time zone ("09:00–18:00"). */
function todayHours(branch: PublicBranch) {
  const today = dateKeyInZone(new Date(), branch.timezone);
  const special = branch.special_days.find((day) => day.date === today);
  if (special)
    return special.closed ? null : `${special.open}–${special.close}`;
  const weekday = (new Date(`${today}T12:00:00Z`).getUTCDay() + 6) % 7;
  const hours = branch.working_hours[WEEKDAYS[weekday]!];
  return !hours || hours.closed ? null : `${hours.open}–${hours.close}`;
}

/** Branch step: shown only when the business has more than one branch. */
export function BranchStep({
  branches,
  value,
  onChange,
}: {
  branches: PublicBranch[];
  value: string;
  onChange: (uuid: string) => void;
}) {
  const { t } = useLocale();
  return (
    <RadioGroup
      value={value}
      onValueChange={onChange}
      aria-label={t("business_page.steps.branch")}
    >
      {branches.map((branch) => {
        const hours = todayHours(branch);
        const address = [branch.address, branch.district, branch.city]
          .filter(Boolean)
          .join(", ");
        return (
          <ChoiceCard
            key={branch.uuid}
            id={`branch-${branch.uuid}`}
            title={branch.name}
            description={
              <>
                {address ? <span className="block">{address}</span> : null}
                <span className="block">
                  {hours
                    ? t("business_page.branch.open_today", { hours })
                    : t("business_page.branch.closed_today")}
                </span>
              </>
            }
            control={
              <RadioGroupItem
                value={branch.uuid}
                id={`branch-${branch.uuid}`}
              />
            }
          />
        );
      })}
    </RadioGroup>
  );
}

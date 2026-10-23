"use client";

import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import {
  linePrice,
  staffCandidates,
} from "@/features/business-page/lib/wizard";
import type {
  PublicService,
  PublicStaff,
} from "@/features/business-page/types";
import { priceLabel } from "@/features/services/lib/format";
import { useLocale } from "@/providers/locale-provider";

/** Radio value of "anyone free" (the slot engine picks a person). */
const ANY = "any";

/**
 * Staff step: per service, a person shown online who performs it, or
 * anyone free. A person's own price / duration is shown when it differs.
 */
export function StaffStep({
  services,
  staff,
  value,
  onChange,
  staffTerm,
  currency,
}: {
  services: PublicService[];
  staff: PublicStaff[];
  value: Record<string, string | null>;
  onChange: (serviceUuid: string, staffUuid: string | null) => void;
  /** Sector word for the person (çalışan, uzman, eğitmen). */
  staffTerm: string;
  currency: string;
}) {
  const { t, locale } = useLocale();
  const staffUuids = staff.map((s) => s.uuid);
  const byUuid = new Map(staff.map((s) => [s.uuid, s]));

  return (
    <div className="space-y-5">
      {services.map((service) => {
        const candidates = staffCandidates(service, staffUuids);
        if (candidates.length === 0) return null;
        const picked = value[service.uuid] ?? ANY;
        return (
          <section key={service.uuid} className="space-y-2">
            {services.length > 1 ? (
              <h3 className="text-sm font-semibold">{service.name}</h3>
            ) : null}
            <RadioGroup
              value={picked}
              onValueChange={(next) =>
                onChange(service.uuid, next === ANY ? null : next)
              }
              aria-label={service.name}
            >
              <ChoiceCard
                id={`staff-${service.uuid}-any`}
                title={t("business_page.staff.any", { term: staffTerm })}
                description={t("business_page.staff.any_hint")}
                control={
                  <RadioGroupItem
                    value={ANY}
                    id={`staff-${service.uuid}-any`}
                  />
                }
              />
              {candidates.map((candidate) => {
                const person = byUuid.get(candidate.staff_uuid);
                if (!person) return null;
                const id = `staff-${service.uuid}-${person.uuid}`;
                const own = linePrice(service, person.uuid);
                const differs =
                  own.price !== service.price ||
                  own.duration !== service.duration_minutes;
                return (
                  <ChoiceCard
                    key={person.uuid}
                    id={id}
                    title={person.name}
                    description={
                      [
                        person.title,
                        differs
                          ? t("business_page.minutes", {
                              count: own.duration,
                            })
                          : "",
                      ]
                        .filter(Boolean)
                        .join(" · ") || undefined
                    }
                    aside={
                      differs
                        ? priceLabel(own.price, currency, locale)
                        : undefined
                    }
                    control={<RadioGroupItem value={person.uuid} id={id} />}
                  />
                );
              })}
            </RadioGroup>
          </section>
        );
      })}
    </div>
  );
}

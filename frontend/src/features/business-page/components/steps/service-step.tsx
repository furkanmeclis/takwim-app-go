"use client";

import { Checkbox } from "@/components/ui/checkbox";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { Badge } from "@/components/ui/badge";
import {
  MAX_LINES,
  groupCapacity,
  toggleService,
} from "@/features/business-page/lib/wizard";
import type { PublicService } from "@/features/business-page/types";
import { priceLabel } from "@/features/services/lib/format";
import { useLocale } from "@/providers/locale-provider";

type Group = { key: string; name: string; services: PublicService[] };

function groupByCategory(services: PublicService[], other: string): Group[] {
  const groups = new Map<string, Group>();
  for (const service of services) {
    const key = service.category?.uuid ?? "";
    const group = groups.get(key) ?? {
      key,
      name: service.category?.name ?? other,
      services: [],
    };
    group.services.push(service);
    groups.set(key, group);
  }
  return [...groups.values()];
}

/**
 * Service step: one or more services of the branch (a combined visit), with
 * the branch's price and duration. The order of picking is the order of the
 * visit. A group class (gym) shows its seats and is booked on its own.
 */
export function ServiceStep({
  services,
  value,
  onChange,
  currency,
}: {
  services: PublicService[];
  value: string[];
  onChange: (uuids: string[]) => void;
  currency: string;
}) {
  const { t, locale } = useLocale();
  const groups = groupByCategory(services, t("business_page.service.other"));
  const full = value.length >= MAX_LINES;
  const groupPicked = value.some(
    (id) => groupCapacity(services.find((s) => s.uuid === id)) !== null,
  );

  const toggle = (uuid: string, checked: boolean) => {
    onChange(toggleService(services, value, uuid, checked));
  };

  return (
    <div className="space-y-5">
      {groups.map((group) => (
        <section key={group.key} className="space-y-2">
          {groups.length > 1 ? (
            <h3 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              {group.name}
            </h3>
          ) : null}
          <div className="grid gap-2">
            {group.services.map((service) => {
              const id = `service-${service.uuid}`;
              const checked = value.includes(service.uuid);
              const seats = groupCapacity(service);
              // A group class replaces the picks, so the limit never
              // blocks it.
              const blocked = !checked && full && seats === null;
              return (
                <ChoiceCard
                  key={service.uuid}
                  id={id}
                  title={
                    seats !== null ? (
                      <span className="flex flex-wrap items-center gap-2">
                        {service.name}
                        <Badge variant="secondary">
                          {t("business_page.service.group", { count: seats })}
                        </Badge>
                      </span>
                    ) : (
                      service.name
                    )
                  }
                  description={
                    <>
                      {service.description ? (
                        <span className="line-clamp-2 block">
                          {service.description}
                        </span>
                      ) : null}
                      <span className="block">
                        {t("business_page.minutes", {
                          count: service.duration_minutes,
                        })}
                      </span>
                    </>
                  }
                  aside={priceLabel(
                    service.price,
                    service.currency || currency,
                    locale,
                  )}
                  disabled={blocked}
                  control={
                    <Checkbox
                      id={id}
                      checked={checked}
                      disabled={blocked}
                      onCheckedChange={(next) =>
                        toggle(service.uuid, next === true)
                      }
                    />
                  }
                />
              );
            })}
          </div>
        </section>
      ))}
      {groupPicked ? (
        <p className="text-muted-foreground text-sm">
          {t("business_page.service.group_alone")}
        </p>
      ) : null}
      {full ? (
        <p className="text-muted-foreground text-sm">
          {t("business_page.service.max", { count: MAX_LINES })}
        </p>
      ) : null}
    </div>
  );
}

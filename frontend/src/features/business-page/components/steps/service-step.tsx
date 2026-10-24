"use client";

import { Checkbox } from "@/components/ui/checkbox";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import { Badge } from "@/components/ui/badge";
import {
  MAX_LINES,
  groupCapacity,
  toggleService,
} from "@/features/business-page/lib/wizard";
import type {
  PublicBundle,
  PublicService,
} from "@/features/business-page/types";
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
 *
 * A combined bundle (TEC-985) is offered first: picking one books its
 * services in the bundle's order at the bundle price; picking a single
 * service drops the bundle, so the two never mix.
 */
export function ServiceStep({
  services,
  value,
  onChange,
  currency,
  bundles = [],
  bundle = "",
  onBundleChange,
}: {
  services: PublicService[];
  value: string[];
  onChange: (uuids: string[]) => void;
  currency: string;
  /** Bundles bookable online in the branch. */
  bundles?: PublicBundle[];
  /** The picked bundle's uuid; empty for single services. */
  bundle?: string;
  onBundleChange?: (uuid: string) => void;
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
  const money = (amount: string, own?: string) =>
    priceLabel(amount, own || currency, locale);

  return (
    <div className="space-y-5">
      {bundles.length > 0 && onBundleChange ? (
        <section className="space-y-2">
          <h3 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {t("business_page.bundle.title")}
          </h3>
          <div className="grid gap-2">
            {bundles.map((item) => {
              const id = `bundle-${item.uuid}`;
              const checked = bundle === item.uuid;
              const saves = Number(item.discount) > 0;
              return (
                <ChoiceCard
                  key={item.uuid}
                  id={id}
                  title={
                    <span className="flex flex-wrap items-center gap-2">
                      {item.name}
                      <Badge variant="secondary">
                        {t("business_page.bundle.items", {
                          count: item.items.length,
                        })}
                      </Badge>
                    </span>
                  }
                  description={
                    <>
                      <span className="block">
                        {item.items.map((i) => i.service_name).join(" + ")}
                      </span>
                      {item.description ? (
                        <span className="line-clamp-2 block">
                          {item.description}
                        </span>
                      ) : null}
                      <span className="block">
                        {t("business_page.minutes", {
                          count: item.duration_minutes,
                        })}
                      </span>
                    </>
                  }
                  aside={
                    <span className="block">
                      {saves ? (
                        <span className="text-muted-foreground block text-xs font-normal line-through">
                          {money(item.list_price, item.currency)}
                        </span>
                      ) : null}
                      <span className="block">
                        {money(item.price, item.currency)}
                      </span>
                      {saves ? (
                        <span className="text-primary block text-xs font-normal">
                          {t("business_page.bundle.save", {
                            amount: money(item.discount, item.currency),
                          })}
                        </span>
                      ) : null}
                    </span>
                  }
                  control={
                    <Checkbox
                      id={id}
                      checked={checked}
                      onCheckedChange={(next) =>
                        onBundleChange(next === true ? item.uuid : "")
                      }
                    />
                  }
                />
              );
            })}
          </div>
          <p className="text-muted-foreground text-sm">
            {t("business_page.bundle.hint")}
          </p>
        </section>
      ) : null}
      {groups.map((group) => (
        <section key={group.key} className="space-y-2">
          {groups.length > 1 || bundles.length > 0 ? (
            <h3 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              {group.name}
            </h3>
          ) : null}
          <div className="grid gap-2">
            {group.services.map((service) => {
              const id = `service-${service.uuid}`;
              const checked = !bundle && value.includes(service.uuid);
              const seats = groupCapacity(service);
              // A group class replaces the picks, so the limit never
              // blocks it.
              const blocked = !bundle && !checked && full && seats === null;
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
                  aside={money(service.price, service.currency)}
                  disabled={blocked}
                  control={
                    <Checkbox
                      id={id}
                      checked={checked}
                      disabled={blocked}
                      onCheckedChange={(next) => {
                        // A single service replaces the bundle.
                        if (bundle) {
                          onBundleChange?.("");
                          onChange(next === true ? [service.uuid] : []);
                          return;
                        }
                        toggle(service.uuid, next === true);
                      }}
                    />
                  }
                />
              );
            })}
          </div>
        </section>
      ))}
      {groupPicked && !bundle ? (
        <p className="text-muted-foreground text-sm">
          {t("business_page.service.group_alone")}
        </p>
      ) : null}
      {full && !bundle ? (
        <p className="text-muted-foreground text-sm">
          {t("business_page.service.max", { count: MAX_LINES })}
        </p>
      ) : null}
    </div>
  );
}

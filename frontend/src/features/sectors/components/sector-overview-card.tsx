"use client";

import { ChevronRight, Shapes, Sparkles } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  SECTOR_LABEL_KEYS,
  SECTOR_MODULE_LABEL_KEYS,
  SECTOR_RESOURCE_LABEL_KEYS,
  SECTOR_STEP_LABEL_KEYS,
} from "@/features/sectors/constants";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useLocale } from "@/providers/locale-provider";

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <p className="text-muted-foreground text-xs font-medium uppercase">
        {title}
      </p>
      {children}
    </div>
  );
}

/**
 * The organization's sector at a glance (TEC-882): the modules it puts
 * first, the default customer booking flow, what a reservation holds and
 * the other visible modules. Nothing renders until the scope loads.
 */
export function SectorOverviewCard() {
  const { t } = useLocale();
  const { sector, term } = useSector();
  if (!sector) return null;

  const others = sector.modules.filter((m) => !sector.highlights.includes(m));
  const staff =
    sector.capacity.staff === "required"
      ? t("sectors.capacity.staff_required", { staff: term("staff") })
      : sector.capacity.staff === "optional"
        ? t("sectors.capacity.staff_optional", { staff: term("staff") })
        : null;
  const resources = sector.capacity.resources
    .map((r) => t(SECTOR_RESOURCE_LABEL_KEYS[r]))
    .join(", ");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Shapes className="size-4" />
          {t("sectors.overview.title", {
            sector: t(SECTOR_LABEL_KEYS[sector.code]),
          })}
        </CardTitle>
        <CardDescription>{t("sectors.overview.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <Section title={t("sectors.overview.highlights")}>
          <div className="flex flex-wrap gap-2">
            {sector.highlights.map((m) => (
              <Badge key={m} className="gap-1">
                <Sparkles className="size-3" />
                {t(SECTOR_MODULE_LABEL_KEYS[m])}
              </Badge>
            ))}
          </div>
        </Section>
        <Section title={t("sectors.overview.flow")}>
          <div className="flex flex-wrap items-center gap-1.5 text-sm">
            {sector.flow.map((step, i) => (
              <Fragment key={step}>
                {i > 0 ? (
                  <ChevronRight className="text-muted-foreground size-3.5" />
                ) : null}
                <Badge variant="outline">
                  {t(SECTOR_STEP_LABEL_KEYS[step])}
                </Badge>
              </Fragment>
            ))}
          </div>
        </Section>
        <Section title={t("sectors.overview.capacity")}>
          <ul className="text-sm">
            {staff ? <li>{staff}</li> : null}
            {resources ? (
              <li>
                {t(
                  sector.capacity.resource_required
                    ? "sectors.capacity.resource_required"
                    : "sectors.capacity.resource_optional",
                  { resources },
                )}
              </li>
            ) : null}
            {sector.capacity.party_size ? (
              <li>{t("sectors.capacity.party_size")}</li>
            ) : null}
            {sector.capacity.group_sessions ? (
              <li>{t("sectors.capacity.group_sessions")}</li>
            ) : null}
          </ul>
        </Section>
        {others.length ? (
          <Section title={t("sectors.overview.modules")}>
            <div className="flex flex-wrap gap-2">
              {others.map((m) => (
                <Badge key={m} variant="secondary">
                  {t(SECTOR_MODULE_LABEL_KEYS[m])}
                </Badge>
              ))}
            </div>
          </Section>
        ) : null}
      </CardContent>
    </Card>
  );
}

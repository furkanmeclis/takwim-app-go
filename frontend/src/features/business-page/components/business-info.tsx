"use client";

import { Globe, Mail, MapPin, Phone } from "lucide-react";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { WEEKDAYS } from "@/features/business-page/constants";
import type {
  PublicBranch,
  PublicBusiness,
} from "@/features/business-page/types";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Hours = PublicBranch["working_hours"][string];

function HoursText({ hours }: { hours?: Hours }) {
  const { t } = useLocale();
  if (!hours || hours.closed) {
    return (
      <span className="text-muted-foreground">
        {t("business_page.info.closed")}
      </span>
    );
  }
  return (
    <span className="tabular-nums">
      {hours.open}–{hours.close}
    </span>
  );
}

/** Weekly hours and the coming special days of a branch. */
export function BranchHours({ branch }: { branch: PublicBranch }) {
  const { t, locale } = useLocale();
  return (
    <div className="space-y-3 text-sm">
      <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1">
        {WEEKDAYS.map((day) => (
          <div key={day} className="contents">
            <dt className="text-muted-foreground">
              {t(`business_page.weekdays.${day}`)}
            </dt>
            <dd className="text-end">
              <HoursText hours={branch.working_hours[day]} />
            </dd>
          </div>
        ))}
      </dl>
      {branch.special_days.length > 0 ? (
        <div className="space-y-1">
          <p className="font-medium">{t("business_page.info.special_days")}</p>
          <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1">
            {branch.special_days.slice(0, 5).map((day) => (
              <div key={day.date} className="contents">
                <dt className="text-muted-foreground">
                  {formatDate(day.date, "d MMMM EEE", locale)}
                  {day.note ? ` · ${day.note}` : ""}
                </dt>
                <dd className="text-end">
                  <HoursText hours={day} />
                </dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}
    </div>
  );
}

function branchAddress(branch: PublicBranch) {
  return [branch.address, branch.district, branch.city]
    .filter(Boolean)
    .join(", ");
}

/** Contact lines (tap to call / write / open the map). */
function ContactLines({
  phone,
  email,
  website,
  address,
}: {
  phone?: string;
  email?: string;
  website?: string;
  address?: string;
}) {
  const site = website
    ? website.startsWith("http")
      ? website
      : `https://${website}`
    : "";
  return (
    <ul className="space-y-2 text-sm">
      {address ? (
        <li className="flex items-start gap-2">
          <MapPin className="text-muted-foreground mt-0.5 size-4 shrink-0" />
          <a
            className="hover:underline"
            href={`https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(address)}`}
            target="_blank"
            rel="noopener noreferrer"
          >
            {address}
          </a>
        </li>
      ) : null}
      {phone ? (
        <li className="flex items-center gap-2">
          <Phone className="text-muted-foreground size-4 shrink-0" />
          <a className="hover:underline" href={`tel:${phone}`}>
            {phone}
          </a>
        </li>
      ) : null}
      {email ? (
        <li className="flex items-center gap-2">
          <Mail className="text-muted-foreground size-4 shrink-0" />
          <a className="break-all hover:underline" href={`mailto:${email}`}>
            {email}
          </a>
        </li>
      ) : null}
      {site ? (
        <li className="flex items-center gap-2">
          <Globe className="text-muted-foreground size-4 shrink-0" />
          <a
            className="break-all hover:underline"
            href={site}
            target="_blank"
            rel="noopener noreferrer"
          >
            {website}
          </a>
        </li>
      ) : null}
    </ul>
  );
}

/**
 * About tab of the business page: branches with hours and contact, the
 * business's contact details and its cancellation / change / no-show
 * policies.
 */
export function BusinessInfo({
  business,
  branches,
}: {
  business: PublicBusiness;
  branches: PublicBranch[];
}) {
  const { t } = useLocale();
  const { contact, policies } = business;
  const policyItems = (
    [
      ["cancellation", policies.cancellation],
      ["change", policies.change],
      ["no_show", policies.no_show],
    ] as const
  ).filter(([, text]) => text.trim());
  const hasContact = Boolean(
    contact.phone || contact.email || contact.website || contact.address,
  );

  return (
    <div className="space-y-4">
      {branches.map((branch) => (
        <Card key={branch.uuid} className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle>{branch.name}</CardTitle>
            {branch.description ? (
              <CardDescription>{branch.description}</CardDescription>
            ) : null}
          </CardHeader>
          <CardContent className="space-y-4 px-4">
            <ContactLines
              address={branchAddress(branch)}
              phone={branch.phone}
              email={branch.email}
            />
            <Accordion type="single" collapsible>
              <AccordionItem value="hours" className="border-b-0">
                <AccordionTrigger className="py-2">
                  {t("business_page.info.hours")}
                </AccordionTrigger>
                <AccordionContent>
                  <BranchHours branch={branch} />
                </AccordionContent>
              </AccordionItem>
            </Accordion>
          </CardContent>
        </Card>
      ))}
      {hasContact ? (
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle>{t("business_page.info.contact")}</CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <ContactLines {...contact} />
          </CardContent>
        </Card>
      ) : null}
      {policyItems.length > 0 ? (
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle>{t("business_page.info.policies")}</CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <Accordion type="multiple">
              {policyItems.map(([key, text]) => (
                <AccordionItem key={key} value={key}>
                  <AccordionTrigger className="py-2">
                    {t(`business_page.policies.${key}`)}
                  </AccordionTrigger>
                  <AccordionContent className="text-muted-foreground text-sm whitespace-pre-line">
                    {text}
                  </AccordionContent>
                </AccordionItem>
              ))}
            </Accordion>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

"use client";

import { Info } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Separator } from "@/components/ui/separator";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import {
  checkoutPrices,
  seatingLabel,
  sumAmounts,
} from "@/features/business-page/lib/wizard";
import type {
  ContactValues,
  PaymentMethod,
  PublicBusiness,
  PublicCheckout,
  PublicService,
} from "@/features/business-page/types";
import { priceLabel } from "@/features/services/lib/format";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Summary step: the held time, the services with their people and prices,
 * the contact, how the customer pays and the business's rules. The prices
 * are the catalog's; the API sets the final amount on the booking.
 */
export function SummaryStep({
  business,
  checkout,
  services,
  contact,
  payments,
  payment,
  onPaymentChange,
}: {
  business: PublicBusiness;
  checkout: PublicCheckout;
  services: PublicService[];
  contact: ContactValues;
  payments: PaymentMethod[];
  payment: PaymentMethod | undefined;
  onPaymentChange: (method: PaymentMethod) => void;
}) {
  const { t, locale } = useLocale();
  const zone = business.timezone;
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });
  const byUuid = new Map(services.map((s) => [s.uuid, s]));
  const prices = checkoutPrices(checkout, services);
  const total = sumAmounts(prices);
  const money = (amount: string | number) =>
    priceLabel(String(amount), business.currency, locale);

  return (
    <div className="space-y-4">
      <Card className="gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle>
            {datetimeInZone(checkout.starts_at, zone, locale, {
              weekday: "long",
              day: "numeric",
              month: "long",
              hour: "2-digit",
              minute: "2-digit",
            })}
          </CardTitle>
          <p className="text-muted-foreground text-sm">
            {[
              checkout.branch.name,
              seatingLabel(t, checkout.party_size, checkout.zone),
            ]
              .filter(Boolean)
              .join(" · ")}
          </p>
        </CardHeader>
        <CardContent className="space-y-3 px-4">
          <ul className="space-y-2 text-sm">
            {checkout.lines.map((line, index) => (
              <li
                key={`${line.service_uuid}-${index}`}
                className="flex justify-between gap-3"
              >
                <span>
                  <span className="block font-medium">
                    {byUuid.get(line.service_uuid)?.name ?? ""}
                  </span>
                  <span className="text-muted-foreground block tabular-nums">
                    {time(line.starts_at)}–{time(line.ends_at)}
                    {line.staff ? ` · ${line.staff.name}` : ""}
                  </span>
                </span>
                <span className="shrink-0 tabular-nums">
                  {money(prices[index] ?? "0")}
                </span>
              </li>
            ))}
          </ul>
          <Separator />
          <p className="flex justify-between text-sm font-semibold">
            <span>{t("business_page.summary.total")}</span>
            <span className="tabular-nums">{money(total.toFixed(2))}</span>
          </p>
        </CardContent>
      </Card>

      <Card className="gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle>{t("business_page.summary.contact")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1 px-4 text-sm">
          <p>{[contact.first_name, contact.last_name].join(" ").trim()}</p>
          <p className="text-muted-foreground">{contact.phone}</p>
          {contact.email ? (
            <p className="text-muted-foreground">{contact.email}</p>
          ) : null}
          {contact.customer_note ? (
            <p className="text-muted-foreground whitespace-pre-line">
              {contact.customer_note}
            </p>
          ) : null}
        </CardContent>
      </Card>

      {payments.length > 0 ? (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">
            {t("business_page.summary.payment")}
          </h3>
          <RadioGroup
            value={payment ?? ""}
            onValueChange={(next) => onPaymentChange(next as PaymentMethod)}
            aria-label={t("business_page.summary.payment")}
          >
            {payments.map((method) => (
              <ChoiceCard
                key={method}
                id={`payment-${method}`}
                title={t(`business_page.payment.${method}`)}
                description={t(`business_page.payment.${method}_hint`)}
                control={
                  <RadioGroupItem value={method} id={`payment-${method}`} />
                }
              />
            ))}
          </RadioGroup>
        </section>
      ) : null}

      {business.booking.approval_mode === "staff" ? (
        <Alert>
          <Info />
          <AlertDescription>
            {t("business_page.summary.approval_note")}
          </AlertDescription>
        </Alert>
      ) : null}
      {business.policies.cancellation.trim() ? (
        <p className="text-muted-foreground text-xs whitespace-pre-line">
          <span className="font-medium">
            {t("business_page.policies.cancellation")}:
          </span>{" "}
          {business.policies.cancellation}
        </p>
      ) : null}
    </div>
  );
}

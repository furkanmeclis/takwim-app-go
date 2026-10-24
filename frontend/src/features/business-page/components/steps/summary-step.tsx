"use client";

import { Info } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { ChoiceCard } from "@/features/business-page/components/choice-card";
import {
  checkoutPrices,
  flowHasParty,
  productKey,
  productPrices,
  seatingLabel,
  sumAmounts,
  vehicleText,
} from "@/features/business-page/lib/wizard";
import type {
  ContactValues,
  PaymentMethod,
  PublicBusiness,
  PublicBookingProduct,
  PublicBundle,
  PublicCheckout,
  PublicProduct,
  PublicService,
  PublicVehicle,
} from "@/features/business-page/types";
import type { CustomerPackage } from "@/features/customer-panel/types";
import { answerText } from "@/features/reservations/lib/custom-fields";

/** Select value for "pay the line" (no package). */
const NO_PACKAGE = "none";
import { priceLabel } from "@/features/services/lib/format";
import { datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Summary step: the held time, the services with their people and prices,
 * the picked products, the vehicle (car wash), the contact, how the customer pays and the business's rules. The prices
 * are the catalog's; the API sets the final amount on the booking.
 */
export function SummaryStep({
  business,
  checkout,
  services,
  products = [],
  pickedProducts = [],
  contact,
  payments,
  payment,
  onPaymentChange,
  vehicle = null,
  packageChoices = [],
  packageBy = {},
  covered = new Set<number>(),
  onPackageChange,
  bundle = null,
}: {
  business: PublicBusiness;
  checkout: PublicCheckout;
  services: PublicService[];
  products?: PublicProduct[];
  pickedProducts?: PublicBookingProduct[];
  contact: ContactValues;
  payments: PaymentMethod[];
  payment: PaymentMethod | undefined;
  onPaymentChange: (method: PaymentMethod) => void;
  vehicle?: PublicVehicle | null;
  /** Per line, the signed-in person's packages that can pay it (TEC-984). */
  packageChoices?: CustomerPackage[][];
  /** Line index → picked package. */
  packageBy?: Record<number, string>;
  /** Lines a picked package pays. */
  covered?: Set<number>;
  onPackageChange?: (line: number, packageUuid: string | null) => void;
  /** The combined bundle the lines belong to (TEC-985). */
  bundle?: PublicBundle | null;
}) {
  const { t, locale } = useLocale();
  const zone = business.timezone;
  const time = (iso: string) =>
    datetimeInZone(iso, zone, locale, { hour: "2-digit", minute: "2-digit" });
  const byUuid = new Map(services.map((s) => [s.uuid, s]));
  const prices = checkoutPrices(checkout, services, bundle);
  const itemPrices = productPrices(products, pickedProducts);
  const bundleSaves = bundle !== null && Number(bundle.discount) > 0;
  const total =
    sumAmounts(prices.filter((_, index) => !covered.has(index))) +
    sumAmounts(itemPrices);
  // A package offered to a line while other lines leave it sessions.
  const offered = (index: number) =>
    (packageChoices[index] ?? []).filter((p) => {
      const others = Object.entries(packageBy).filter(
        ([line, id]) => Number(line) !== index && id === p.uuid,
      ).length;
      return p.remaining > others;
    });
  const itemName = (item: PublicBookingProduct) => {
    const p = products.find((x) => productKey(x) === productKey(item));
    if (!p) return "";
    return p.variant_name ? `${p.name} / ${p.variant_name}` : p.name;
  };
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
              seatingLabel(
                t,
                checkout.party_size,
                checkout.zone,
                flowHasParty(business.flow),
              ),
              vehicleText(t, vehicle),
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
                  {line.bundle ? (
                    <span className="text-muted-foreground block text-xs">
                      {t("business_page.bundle.in_bundle", {
                        name: line.bundle.name,
                      })}
                    </span>
                  ) : null}
                  {onPackageChange && offered(index).length > 0 ? (
                    <Select
                      value={
                        covered.has(index) && packageBy[index]
                          ? packageBy[index]
                          : NO_PACKAGE
                      }
                      onValueChange={(next) =>
                        onPackageChange(
                          index,
                          next === NO_PACKAGE ? null : next,
                        )
                      }
                    >
                      <SelectTrigger
                        size="sm"
                        className="mt-2 w-full max-w-72"
                        aria-label={t("business_page.packages.use")}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_PACKAGE}>
                          {t("business_page.packages.none")}
                        </SelectItem>
                        {offered(index).map((p) => (
                          <SelectItem key={p.uuid} value={p.uuid}>
                            {t("business_page.packages.option", {
                              name: p.name,
                              remaining: p.remaining,
                              total: p.total_sessions,
                            })}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : null}
                </span>
                <span className="shrink-0 tabular-nums">
                  {covered.has(index)
                    ? t("business_page.packages.session")
                    : money(prices[index] ?? "0")}
                </span>
              </li>
            ))}
            {pickedProducts.map((item, index) => (
              <li key={productKey(item)} className="flex justify-between gap-3">
                <span>
                  <span className="block font-medium">{itemName(item)}</span>
                  <span className="text-muted-foreground block tabular-nums">
                    {t("business_page.products.line", {
                      quantity: item.quantity,
                    })}
                  </span>
                </span>
                <span className="shrink-0 tabular-nums">
                  {money(itemPrices[index] ?? "0")}
                </span>
              </li>
            ))}
          </ul>
          <Separator />
          {bundle && bundleSaves ? (
            <>
              <p className="text-muted-foreground flex justify-between text-sm">
                <span>{t("business_page.bundle.list_price")}</span>
                <span className="tabular-nums line-through">
                  {money(bundle.list_price)}
                </span>
              </p>
              <p className="text-muted-foreground flex justify-between text-sm">
                <span>
                  {t("business_page.bundle.discount")} · {bundle.name}
                </span>
                <span className="tabular-nums">−{money(bundle.discount)}</span>
              </p>
            </>
          ) : null}
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
          {business.booking.fields.map((field) => {
            const value = contact.custom_fields?.[field.key];
            if (value === undefined || value === "") return null;
            if (field.type === "checkbox" && value !== true) return null;
            return (
              <p key={field.key} className="text-muted-foreground">
                <span className="text-foreground font-medium">
                  {field.label}:
                </span>{" "}
                {answerText({ type: field.type, value }, t)}
              </p>
            );
          })}
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

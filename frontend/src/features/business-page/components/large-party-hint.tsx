"use client";

import { Phone } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Online bookings seat a party at one table and never join tables (the
 * business does that on the panel). When no time fits the party, the
 * customer is pointed to the business's phone.
 */
export function LargePartyHint({
  partySize,
  phone,
}: {
  partySize: number;
  phone?: string;
}) {
  const { t } = useLocale();
  return (
    <Alert>
      <Phone />
      <AlertTitle>{t("business_page.party.call_title")}</AlertTitle>
      <AlertDescription className="space-y-3">
        <p>
          {phone
            ? t("business_page.party.call_description", {
                count: partySize,
                phone,
              })
            : t("business_page.party.call_description_no_phone", {
                count: partySize,
              })}
        </p>
        {phone ? (
          <Button asChild size="sm" variant="outline">
            <a href={`tel:${phone}`}>
              <Phone className="size-4" />
              {t("business_page.party.call_action")}
            </a>
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}
